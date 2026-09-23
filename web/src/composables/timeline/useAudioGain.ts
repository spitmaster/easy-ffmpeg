import { watch, type Ref } from 'vue'

/**
 * WebAudio gain pipeline for an HTMLAudioElement, used to allow volumes
 * above 100% (HTMLMediaElement.volume hard-caps at 1.0). The GainNode is
 * created lazily on the first apply() call so callers can wire this up at
 * setup time without forcing AudioContext creation before user interaction.
 *
 * The composable owns the AudioContext + GainNode for the lifetime of the
 * passed audioRef. When WebAudio is unavailable or initialization fails,
 * apply() falls back to clamping element.volume to 1.0 and logs a warning.
 *
 * volume() is the desired gain (0–2.0+ typical); the composable also wires
 * a watch so changes to the underlying source propagate automatically.
 */
export function useAudioGain(
  audioRef: Ref<HTMLAudioElement | null>,
  volume: () => number,
) {
  let ctx: AudioContext | null = null
  let gainNode: GainNode | null = null

  function initGainNode() {
    const a = audioRef.value
    if (!a || gainNode) return
    type AudioCtor = typeof AudioContext
    const w = window as Window & { webkitAudioContext?: AudioCtor }
    const Ctor: AudioCtor | undefined = window.AudioContext || w.webkitAudioContext
    if (!Ctor) return
    try {
      ctx = new Ctor()
      const src = ctx.createMediaElementSource(a)
      gainNode = ctx.createGain()
      gainNode.gain.value = 1
      src.connect(gainNode).connect(ctx.destination)
    } catch (e) {
      console.warn('[timeline] WebAudio gain unavailable; preview volume capped at 100%:', e)
      ctx = null
      gainNode = null
    }
  }

  function apply() {
    const a = audioRef.value
    if (!a) return
    const v = Math.max(0, volume() ?? 1)
    if (!gainNode) initGainNode()
    if (gainNode) {
      gainNode.gain.value = v
      a.volume = 1
      if (ctx && ctx.state === 'suspended') {
        ctx.resume().catch(() => {})
      }
    } else {
      a.volume = Math.min(1, v)
    }
  }

  // Auto-apply on volume changes from the caller's source of truth.
  watch(volume, () => apply())

  return { apply }
}
