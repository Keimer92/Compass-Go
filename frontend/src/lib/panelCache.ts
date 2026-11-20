import { gitStatus, gitBranch, listDir, gitLog } from './bridge'
import type { Change, FileNode, CommitEntry } from './bridge'

// Cache de los paneles laterales por proyecto.
//
// Igual que gitCache: al cambiar de pestaña el componente se desmonta y vuelve
// a consultar todo (varios procesos git + recorridos de disco), lo que en
// Windows deja la pestaña vacia un par de segundos. Guardando el ultimo
// resultado la pestaña aparece ya pintada y se refresca en segundo plano.

export interface ExplorerSnapshot {
  changes: Change[]
  branch: string
  roots: FileNode[]
  children: Record<string, FileNode[]>
  expanded: string[]
}

const explorer = new Map<string, ExplorerSnapshot>()
const log = new Map<string, CommitEntry[]>()

export function peekExplorer(project: string): ExplorerSnapshot | null {
  return explorer.get(project) ?? null
}

export function saveExplorer(project: string, snap: ExplorerSnapshot) {
  if (project) explorer.set(project, snap)
}

export function peekLog(project: string): CommitEntry[] | null {
  return log.get(project) ?? null
}

export function saveLog(project: string, commits: CommitEntry[]) {
  if (project) log.set(project, commits)
}

// Precalientan el cache al activar un proyecto, para que la primera visita a
// cada pestaña tampoco espere. Si ya hay datos no hacen nada.
export async function warmExplorer(project: string) {
  if (!project || explorer.has(project)) return
  const [changes, branch, roots] = await Promise.all([gitStatus(), gitBranch(), listDir('')])
  saveExplorer(project, { changes, branch, roots, children: {}, expanded: [] })
}

export async function warmLog(project: string) {
  if (!project || log.has(project)) return
  saveLog(project, await gitLog())
}
