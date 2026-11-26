// Estado de push/sync persistente por proyecto y desacoplado del componente:
// el panel Git se desmonta al cambiar de pestaña, pero la operación sigue
// corriendo en Go. El resultado se escribe acá y el toast se muestra aunque
// el panel ya no esté montado. Mismo patrón que commitStore.svelte.ts.
import { gitPush, gitSync, onPushProgress, type PushProgress } from './bridge'
import { forgetGitInfo } from './gitCache'
import { notifyRepoChanged } from './repoChanged'
import { pushToast } from './toast.svelte'

export type GitAction = '' | 'push' | 'sync'

export interface GitRun {
  action: GitAction
  busy: boolean
  notice: string
  error: string
  progress: PushProgress | null
}

const runs = $state<Record<string, GitRun>>({})

const EMPTY_RUN: GitRun = {
  action: '',
  busy: false,
  notice: '',
  error: '',
  progress: null
}

// Lectura pura: se usa desde un $derived, así que no muta el store.
export function taskFor(path: string): GitRun {
  return runs[path] ?? EMPTY_RUN
}

function ensureTask(path: string): GitRun {
  if (!runs[path]) runs[path] = { ...EMPTY_RUN }
  return runs[path]
}

function samePath(a: string, b: string) {
  if (!a || !b) return false
  return a.replace(/\\/g, '/').toLowerCase() === b.replace(/\\/g, '/').toLowerCase()
}

let progressStarted = false

// Una sola suscripción global: Go incluye el directorio en cada evento, así
// que el avance se enruta al proyecto correcto aunque no sea el activo.
export function initTaskProgress() {
  if (progressStarted) return
  progressStarted = true
  onPushProgress((p) => {
    if (!p.dir) return
    for (const key of Object.keys(runs)) {
      if (samePath(key, p.dir)) {
        runs[key].progress = p
        return
      }
    }
  })
}

export async function runPush(
  path: string,
  action: 'push' | 'sync',
  opts: { silent?: boolean } = {}
): Promise<string> {
  const state = ensureTask(path)
  if (state.busy) return ''
  state.busy = true
  state.action = action
  state.notice = ''
  state.error = ''
  state.progress = null
  try {
    const msg = action === 'push' ? await gitPush() : await gitSync()
    state.notice = msg
    forgetGitInfo(path)
    notifyRepoChanged()
    if (!opts.silent) {
      pushToast({
        kind: 'success',
        title: action === 'push' ? 'Push completado' : 'Sincronización completada',
        detail: msg
      })
    }
    return msg
  } catch (e) {
    const msg = e instanceof Error ? e.message : String(e)
    state.error = msg
    if (opts.silent) throw e
    pushToast({
      kind: 'error',
      title: action === 'push' ? 'Push fallido' : 'Sincronización fallida',
      detail: msg
    })
    return ''
  } finally {
    state.busy = false
    state.action = ''
    state.progress = null
  }
}
