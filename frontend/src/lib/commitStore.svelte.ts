// Estado del panel de commit, persistente por proyecto y desacoplado del
// componente: cambiar de proyecto (o de pestaña lateral) no debe borrar el
// mensaje generado ni cancelar una generación en curso.
import { generateCommit, cancelCommit, commit } from './bridge'
import { notifyRepoChanged } from './repoChanged'

const STORAGE_KEY = 'compass.go.commit.drafts.v1'

export type CommitDraft = {
  message: string
  lastMs: number
}

type RunState = {
  loading: boolean
  elapsedMs: number
  committing: boolean
  error: string
  ok: string
}

function loadDrafts(): Record<string, CommitDraft> {
  if (typeof localStorage === 'undefined') return {}
  try {
    const raw = localStorage.getItem(STORAGE_KEY)
    if (!raw) return {}
    const parsed: unknown = JSON.parse(raw)
    if (!parsed || typeof parsed !== 'object') return {}
    const out: Record<string, CommitDraft> = {}
    for (const [path, value] of Object.entries(parsed as Record<string, unknown>)) {
      if (!value || typeof value !== 'object') continue
      const d = value as Partial<CommitDraft>
      out[path] = {
        message: typeof d.message === 'string' ? d.message : '',
        lastMs: typeof d.lastMs === 'number' ? d.lastMs : 0
      }
    }
    return out
  } catch {
    return {}
  }
}

const drafts = $state<Record<string, CommitDraft>>(loadDrafts())

function persist() {
  if (typeof localStorage === 'undefined') return
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(drafts))
  } catch {
    // Almacenamiento bloqueado (modo privado): se pierde al cerrar, pero el
    // comportamiento dentro de la sesión sigue siendo correcto.
  }
}

export function draftFor(path: string): CommitDraft {
  return drafts[path] ?? { message: '', lastMs: 0 }
}

export function setMessage(path: string, message: string) {
  const previous = drafts[path] ?? { message: '', lastMs: 0 }
  drafts[path] = { ...previous, message }
  persist()
}

const runs = $state<Record<string, RunState>>({})
const timers: Record<string, ReturnType<typeof setInterval> | undefined> = {}

const EMPTY_RUN: RunState = {
  loading: false,
  elapsedMs: 0,
  committing: false,
  error: '',
  ok: ''
}

// Lectura pura: se usa desde un $derived en el componente, así que no debe
// escribir en el store (Svelte 5 prohibe mutar estado dentro de un derived).
export function runFor(path: string): RunState {
  return runs[path] ?? EMPTY_RUN
}

// Creación diferida: solo se llama desde los handlers de generate/commit.
function ensureRun(path: string): RunState {
  if (!runs[path]) {
    // Al asignarlo dentro del objeto $state, Svelte lo convierte en proxificado.
    runs[path] = { ...EMPTY_RUN }
  }
  return runs[path]
}

export async function generateFor(path: string) {
  const state = ensureRun(path)
  if (state.loading) return
  state.loading = true
  state.error = ''
  state.ok = ''
  state.elapsedMs = 0
  const start = performance.now()
  timers[path] = setInterval(() => {
    state.elapsedMs = performance.now() - start
  }, 100)

  try {
    const message = await generateCommit()
    const previous = drafts[path] ?? { message: '', lastMs: 0 }
    drafts[path] = { message, lastMs: performance.now() - start }
    persist()
    state.ok = 'Mensaje generado'
  } catch (e) {
    const msg = (e as Error).message
    if (msg.includes('cancelada')) state.ok = 'Generación cancelada'
    else state.error = msg
  } finally {
    if (timers[path]) clearInterval(timers[path])
    timers[path] = undefined
    state.loading = false
  }
}

export async function cancelFor() {
  try {
    await cancelCommit()
  } catch {
    // Si ya había terminado, no hay nada que cancelar.
  }
}

export async function commitFor(path: string) {
  const state = ensureRun(path)
  const draft = drafts[path]
  const message = draft?.message?.trim() ?? ''
  if (!message) return
  state.committing = true
  state.error = ''
  state.ok = ''
  try {
    await commit(message)
    state.ok = 'Commit realizado correctamente'
    drafts[path] = { message: '', lastMs: draft.lastMs }
    persist()
    notifyRepoChanged()
  } catch (e) {
    state.error = (e as Error).message
  } finally {
    state.committing = false
  }
}
