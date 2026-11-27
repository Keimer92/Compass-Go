// Package webui — panel GUI del versionador.
//
// La card "Version" ya no abre una ConPTY: el frontend pide el plan (archivos
// de version y su valor actual) y aplica la version nueva desde un modal, igual
// que el resto de acciones con GUI. La logica de deteccion y escritura sigue
// viviendo en tasks.VersionInfo/VersionApply, compartida con la tarea de
// terminal.
package webui

import (
	"errors"

	"compass/internal/tasks"
)

// VersionInfo devuelve los archivos de version del proyecto activo y su valor
// actual. No escribe nada.
func (b *Bridge) VersionInfo() (tasks.VersionPlanInfo, error) {
	b.mu.Lock()
	ctx := b.ctx
	b.mu.Unlock()
	if ctx == nil {
		return tasks.VersionPlanInfo{}, errors.New("no hay proyecto activo")
	}
	return tasks.VersionInfo(ctx)
}

// ApplyVersion escribe la version nueva en todos los archivos del proyecto
// activo. Valida el formato y no deja nada a medias: el plan se calcula entero
// antes de tocar disco.
func (b *Bridge) ApplyVersion(version string) (tasks.VersionResult, error) {
	b.mu.Lock()
	ctx := b.ctx
	b.mu.Unlock()
	if ctx == nil {
		return tasks.VersionResult{}, errors.New("no hay proyecto activo")
	}
	res, err := tasks.VersionApply(ctx, version)
	if err != nil {
		return tasks.VersionResult{}, err
	}
	return res, nil
}
