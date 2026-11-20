import { gitInfo, type GitInfo } from './bridge'

// Cache del ultimo GitInfo por proyecto.
//
// El panel Git se desmonta al cambiar a Explorador/Historial, y `GitInfo()` en
// Go lanza varias decenas de procesos git (rev-parse, remote, rev-list,
// status...), lo que en Windows tarda lo suficiente para que el panel pinte su
// estado inicial. Con este cache la pestana aparece ya con los datos reales y
// la consulta se refresca en segundo plano.
const cache = new Map<string, GitInfo>()

export function emptyGitInfo(): GitInfo {
  return {
    isRepo: false, hasCommits: false, branch: '', upstream: '', remoteUrl: '',
    webUrl: '', isGitHub: false, ahead: 0, behind: 0, changes: 0
  }
}

export function peekGitInfo(project: string): GitInfo | null {
  return cache.get(project) ?? null
}

export async function loadGitInfo(project: string): Promise<GitInfo> {
  const info = await gitInfo()
  if (project) cache.set(project, info)
  return info
}

export function forgetGitInfo(project: string) {
  cache.delete(project)
}
