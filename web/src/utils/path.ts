/**
 * Forward-slash path utilities. The Go backend always returns and accepts
 * paths with `/` even on Windows (filepath.ToSlash on the way out, the
 * driver normalizes back on the way in). Keeping the same convention on
 * the front-end means we never have to think about separators.
 */
export const Path = {
  join(dir: string, name: string): string {
    if (!dir) return name
    return dir.endsWith('/') ? dir + name : dir + '/' + name
  },
  basename(p: string): string {
    const i = p.lastIndexOf('/')
    return i >= 0 ? p.slice(i + 1) : p
  },
  dirname(p: string): string {
    const i = p.lastIndexOf('/')
    return i >= 0 ? p.slice(0, i) : ''
  },
  stripExt(name: string): string {
    const i = name.lastIndexOf('.')
    return i > 0 ? name.slice(0, i) : name
  },
}
