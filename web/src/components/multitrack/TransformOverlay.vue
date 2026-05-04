<script setup lang="ts">
import { computed, ref } from 'vue'
import type { MultitrackCanvas, MultitrackTransform } from '@/api/multitrack'

/**
 * Selection box drawn on top of the preview canvas (v0.5.1 / M5). Visualises
 * a video clip's Transform (X / Y / W / H in canvas pixels) and lets the
 * user move / resize it with eight handles + a centre drag region.
 *
 * Coordinate model:
 *   - The component is absolutely positioned to fill `.preview-canvas`.
 *   - Canvas pixels (transform) → DOM pixels (handle positions) via the
 *     ratio  scale = boxW / canvasW. boxW comes from the parent's
 *     ResizeObserver (canvasBoxStyle pixel size), canvasW from props.
 *   - All commits are integer pixels in canvas space (round to nearest);
 *     fractional drags accumulate inside the local draft so a slow 0.3-px
 *     drag still eventually moves the box.
 *
 * Modifier keys (mirrors the spec in program.md §12.4.2):
 *   - Centre drag: moves (X, Y); Shift locks to single axis (the larger
 *     accumulated delta wins).
 *   - Corner handles: change W and H together; Shift locks aspect ratio
 *     (preferring `sourceRatio` if provided so the user can recover the
 *     source's original ratio after a free transform; falls back to the
 *     transform's ratio at drag start). Alt anchors at centre (W and H
 *     change symmetrically, X/Y compensate so centre stays put).
 *   - Edge handles: change one dimension; Shift / Alt rules same as
 *     corner but apply to the active axis only.
 *
 * Emit contract:
 *   - 'update': emitted on every pointermove with the latest draft. Parent
 *     wires this to store.previewClipTransform (no history push).
 *   - 'commit': emitted on pointerup with the final integer transform.
 *     Parent wires this to store.commitClipTransform (history + dirty).
 *
 * Out-of-bounds: the box can be dragged anywhere; the export pipeline
 * accepts negative X/Y or W/H beyond canvas. Validation only enforces
 * W > 0 and H > 0; we clamp those at drag time.
 */

interface Props {
  canvas: MultitrackCanvas
  transform: MultitrackTransform
  /**
   * Original source aspect ratio (width / height). When provided and > 0,
   * Shift-drag locks against this rather than the live transform's ratio,
   * so a user who has freely deformed a clip can re-acquire its original
   * shape simply by holding Shift while dragging a handle. Optional —
   * the parent may not have source dimensions for every clip.
   */
  sourceRatio?: number
}

const props = defineProps<Props>()

const emit = defineEmits<{
  (e: 'update', t: MultitrackTransform): void
  (e: 'commit', t: MultitrackTransform): void
}>()

type Handle = 'move' | 'n' | 's' | 'e' | 'w' | 'ne' | 'nw' | 'se' | 'sw'

interface DragState {
  handle: Handle
  pointerId: number
  startClientX: number
  startClientY: number
  startTransform: MultitrackTransform
  /** Live draft (canvas pixels, may be fractional during drag). */
  draft: MultitrackTransform
}

const drag = ref<DragState | null>(null)

// DOM scaling: the overlay fills .preview-canvas, so its bounding rect is
// our "canvas-box pixels". scale converts canvas-px → DOM-px for handle
// placement, and 1/scale converts mouse delta back.
const overlayEl = ref<HTMLDivElement | null>(null)

function canvasScale(): number {
  const el = overlayEl.value
  if (!el || props.canvas.width <= 0) return 1
  return el.clientWidth / props.canvas.width
}

const boxStyle = computed(() => {
  // Use draft during drag for sub-pixel-smooth visual feedback; fall back
  // to props (which the parent updates via store on @update) otherwise.
  const t = drag.value?.draft ?? props.transform
  const cw = Math.max(1, props.canvas.width)
  const ch = Math.max(1, props.canvas.height)
  return {
    left: `${(t.x / cw) * 100}%`,
    top: `${(t.y / ch) * 100}%`,
    width: `${(t.w / cw) * 100}%`,
    height: `${(t.h / ch) * 100}%`,
  }
})

function snapshotTransform(): MultitrackTransform {
  return {
    x: Math.round(props.transform.x),
    y: Math.round(props.transform.y),
    w: Math.round(props.transform.w),
    h: Math.round(props.transform.h),
  }
}

function onHandleDown(handle: Handle, ev: PointerEvent) {
  if (ev.button !== 0) return
  ev.preventDefault()
  ev.stopPropagation()
  const target = ev.currentTarget as HTMLElement
  target.setPointerCapture(ev.pointerId)
  const startTransform = snapshotTransform()
  drag.value = {
    handle,
    pointerId: ev.pointerId,
    startClientX: ev.clientX,
    startClientY: ev.clientY,
    startTransform,
    draft: { ...startTransform },
  }
  // Listen on the target since we have pointer capture.
  target.addEventListener('pointermove', onPointerMove)
  target.addEventListener('pointerup', onPointerUp)
  target.addEventListener('pointercancel', onPointerUp)
}

function onPointerMove(ev: PointerEvent) {
  const state = drag.value
  if (!state || ev.pointerId !== state.pointerId) return
  const scale = canvasScale()
  if (scale <= 0) return
  const dxPx = (ev.clientX - state.startClientX) / scale
  const dyPx = (ev.clientY - state.startClientY) / scale
  const next = computeDraft(state, dxPx, dyPx, ev.shiftKey, ev.altKey)
  state.draft = next
  emit('update', roundTransform(next))
}

function onPointerUp(ev: PointerEvent) {
  const state = drag.value
  if (!state || ev.pointerId !== state.pointerId) return
  const target = ev.currentTarget as HTMLElement
  try {
    target.releasePointerCapture(ev.pointerId)
  } catch {
    // Already released — fine.
  }
  target.removeEventListener('pointermove', onPointerMove)
  target.removeEventListener('pointerup', onPointerUp)
  target.removeEventListener('pointercancel', onPointerUp)
  const final = roundTransform(state.draft)
  drag.value = null
  emit('commit', final)
}

function roundTransform(t: MultitrackTransform): MultitrackTransform {
  // W and H must stay strictly positive (Validate enforces > 0).
  return {
    x: Math.round(t.x),
    y: Math.round(t.y),
    w: Math.max(1, Math.round(t.w)),
    h: Math.max(1, Math.round(t.h)),
  }
}

/**
 * Compute the candidate transform for the current drag delta. Pure: takes
 * the start transform, the canvas-px delta, and modifier flags; returns
 * the next draft transform (still fractional).
 *
 * Edge cases:
 *   - W or H going ≤ 0 is clamped to 1 px so the overlay never collapses.
 *     The user can re-grow from a thin sliver instead of "vanishing".
 *   - Shift on corners locks aspect ratio against the START ratio. The
 *     larger axis-delta (in screen px) wins so the user feels in control.
 *   - Alt on corners / edges anchors the opposite side; the box grows /
 *     shrinks symmetrically around its centre (or the active edge's
 *     opposite edge for single-axis handles).
 */
function computeDraft(
  state: DragState,
  dx: number,
  dy: number,
  shift: boolean,
  alt: boolean,
): MultitrackTransform {
  const s = state.startTransform
  const startRatio = s.h > 0 ? s.w / s.h : 1
  // Prefer the source's original ratio when known, so Shift-drag can
  // recover the original shape after a free transform; fall back to the
  // current ratio so the lock still works for clips with unknown source
  // dimensions.
  const lockRatio =
    props.sourceRatio && props.sourceRatio > 0 ? props.sourceRatio : startRatio
  let { x, y, w, h } = s

  switch (state.handle) {
    case 'move': {
      let mx = dx
      let my = dy
      if (shift) {
        if (Math.abs(dx) > Math.abs(dy)) my = 0
        else mx = 0
      }
      x = s.x + mx
      y = s.y + my
      break
    }

    case 'e':
    case 'w':
    case 'n':
    case 's': {
      const isHoriz = state.handle === 'e' || state.handle === 'w'
      const isStart = state.handle === 'w' || state.handle === 'n'
      // Delta along the active axis: positive = grow this side outward.
      const delta = isHoriz ? dx : dy
      if (isHoriz) {
        if (alt) {
          // Symmetric: left and right move equally. dx on 'e' grows width
          // by 2*dx (right side +dx, left side -dx); dx on 'w' shrinks by
          // 2*dx (left side +dx, right side -dx).
          const sign = isStart ? -1 : 1
          w = s.w + 2 * sign * delta
          x = s.x - sign * delta
        } else if (isStart) {
          // 'w' handle: dragging right (positive dx) moves the left edge
          // right, shrinking width.
          x = s.x + delta
          w = s.w - delta
        } else {
          // 'e' handle: dragging right grows width.
          w = s.w + delta
        }
      } else {
        if (alt) {
          const sign = isStart ? -1 : 1
          h = s.h + 2 * sign * delta
          y = s.y - sign * delta
        } else if (isStart) {
          y = s.y + delta
          h = s.h - delta
        } else {
          h = s.h + delta
        }
      }
      // Shift on edges: keep aspect ratio against `lockRatio` (source
      // ratio if known, else the transform's ratio at drag start). The
      // OTHER axis follows, anchored at the opposite edge (or centre when
      // alt is also held).
      if (shift) {
        if (isHoriz) {
          const newH = lockRatio > 0 ? w / lockRatio : h
          if (alt) {
            y = s.y + (s.h - newH) / 2
          }
          h = newH
        } else {
          const newW = h * lockRatio
          if (alt) {
            x = s.x + (s.w - newW) / 2
          }
          w = newW
        }
      }
      break
    }

    case 'ne':
    case 'nw':
    case 'se':
    case 'sw': {
      const isLeft = state.handle === 'nw' || state.handle === 'sw'
      const isTop = state.handle === 'nw' || state.handle === 'ne'
      let dwLocal = isLeft ? -dx : dx
      let dhLocal = isTop ? -dy : dy
      if (shift && lockRatio > 0) {
        // Orthogonal projection of the raw delta (dwLocal, dhLocal) onto
        // the line dw = lockRatio * dh, i.e. the set of (dw, dh) pairs
        // that preserve the locked aspect ratio.
        //
        // The previous implementation picked a "dominant axis" by
        // comparing |dwLocal| vs |dhLocal| and let the other axis follow.
        // That switch is discontinuous at the diagonal |dw| == |dh|: each
        // time the mouse crossed it, the constrained pair jumped from
        // one curve to another, which the user perceived as the box
        // sticking at certain sizes and then catching up. Projection is
        // C¹ continuous everywhere → smooth resize for any mouse path.
        const r = lockRatio
        const k = (r * dwLocal + dhLocal) / (r * r + 1)
        dwLocal = k * r
        dhLocal = k
      }
      if (alt) {
        // Symmetric around centre.
        w = s.w + 2 * dwLocal
        h = s.h + 2 * dhLocal
        x = s.x - dwLocal
        y = s.y - dhLocal
      } else {
        w = s.w + dwLocal
        h = s.h + dhLocal
        if (isLeft) x = s.x - dwLocal
        if (isTop) y = s.y - dhLocal
      }
      break
    }
  }

  // Clamp positive dimensions; preserve anchor for left/top so the box
  // never visually flips when the user drags past zero.
  if (w < 1) {
    if (state.handle === 'w' || state.handle === 'nw' || state.handle === 'sw') {
      x = x + w - 1
    }
    w = 1
  }
  if (h < 1) {
    if (state.handle === 'n' || state.handle === 'nw' || state.handle === 'ne') {
      y = y + h - 1
    }
    h = 1
  }
  return { x, y, w, h }
}
</script>

<template>
  <div
    ref="overlayEl"
    class="pointer-events-none absolute inset-0 z-30"
  >
    <!-- Selection box. pointer-events:auto on this and the handles only;
         the rest of the overlay stays click-through so the playhead /
         scrub interactions in the canvas underneath still receive events
         when the user clicks outside the box. -->
    <div
      class="absolute pointer-events-auto cursor-move border border-accent shadow-[0_0_0_1px_rgba(0,0,0,0.4)]"
      :style="boxStyle"
      @pointerdown="onHandleDown('move', $event)"
    >
      <!-- Edge handles -->
      <div
        class="absolute -top-[5px] left-1/2 h-[10px] w-3 -translate-x-1/2 cursor-ns-resize border border-accent bg-bg-base"
        @pointerdown.stop="onHandleDown('n', $event)"
      />
      <div
        class="absolute -bottom-[5px] left-1/2 h-[10px] w-3 -translate-x-1/2 cursor-ns-resize border border-accent bg-bg-base"
        @pointerdown.stop="onHandleDown('s', $event)"
      />
      <div
        class="absolute top-1/2 -left-[5px] h-3 w-[10px] -translate-y-1/2 cursor-ew-resize border border-accent bg-bg-base"
        @pointerdown.stop="onHandleDown('w', $event)"
      />
      <div
        class="absolute top-1/2 -right-[5px] h-3 w-[10px] -translate-y-1/2 cursor-ew-resize border border-accent bg-bg-base"
        @pointerdown.stop="onHandleDown('e', $event)"
      />

      <!-- Corner handles -->
      <div
        class="absolute -top-[6px] -left-[6px] h-3 w-3 cursor-nwse-resize rounded-full border border-accent bg-bg-base"
        @pointerdown.stop="onHandleDown('nw', $event)"
      />
      <div
        class="absolute -top-[6px] -right-[6px] h-3 w-3 cursor-nesw-resize rounded-full border border-accent bg-bg-base"
        @pointerdown.stop="onHandleDown('ne', $event)"
      />
      <div
        class="absolute -bottom-[6px] -left-[6px] h-3 w-3 cursor-nesw-resize rounded-full border border-accent bg-bg-base"
        @pointerdown.stop="onHandleDown('sw', $event)"
      />
      <div
        class="absolute -bottom-[6px] -right-[6px] h-3 w-3 cursor-nwse-resize rounded-full border border-accent bg-bg-base"
        @pointerdown.stop="onHandleDown('se', $event)"
      />
    </div>
  </div>
</template>
