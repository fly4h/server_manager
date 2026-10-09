package api

import (
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

func (a *API) dockerFail(w http.ResponseWriter, r *http.Request, err error) {
	writeErr(w, r, http.StatusBadGateway, "DOCKER_ERROR", dockerErrMsg(err), nil)
}

func dockerErrMsg(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	if len(msg) > 300 {
		msg = msg[:300]
	}
	return msg
}

func (a *API) handleDockerStatus(w http.ResponseWriter, r *http.Request) {
	if a.docker == nil {
		writeErr(w, r, http.StatusServiceUnavailable, "DOCKER_UNAVAILABLE", "docker client not configured", nil)
		return
	}
	ctx, cancel := contextWithTimeout(r)
	defer cancel()
	if err := a.docker.Ping(ctx); err != nil {
		writeErr(w, r, http.StatusBadGateway, "DOCKER_ERROR", dockerErrMsg(err), nil)
		return
	}
	writeData(w, r, http.StatusOK, map[string]string{"status": "ok"})
}

func (a *API) handleDockerContainers(w http.ResponseWriter, r *http.Request) {
	if a.docker == nil {
		writeErr(w, r, http.StatusServiceUnavailable, "DOCKER_UNAVAILABLE", "docker client not configured", nil)
		return
	}
	ctx, cancel := contextWithTimeout(r)
	defer cancel()
	containers, err := a.docker.ListContainers(ctx, r.URL.Query().Get("all") == "1")
	if err != nil {
		a.dockerFail(w, r, err)
		return
	}
	type row struct {
		ID     string `json:"id"`
		Name   string `json:"name"`
		Image  string `json:"image"`
		State  string `json:"state"`
		Status string `json:"status"`
	}
	out := make([]row, 0, len(containers))
	for _, c := range containers {
		out = append(out, row{ID: c.ID[:12], Name: c.Name(), Image: c.Image, State: c.State, Status: c.Status})
	}
	writeData(w, r, http.StatusOK, out)
}

func (a *API) dockerContainerAction(action string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		s := currentSession(r)
		if a.docker == nil {
			writeErr(w, r, http.StatusServiceUnavailable, "DOCKER_UNAVAILABLE", "docker client not configured", nil)
			return
		}
		id := chi.URLParam(r, "id")
		ctx, cancel := contextWithTimeout(r)
		defer cancel()
		if err := a.docker.ContainerAction(ctx, id, action); err != nil {
			a.dockerFail(w, r, err)
			return
		}
		a.activity.Log(s.UserID, s.Username, "docker_"+action, "container", id, nil)
		writeData(w, r, http.StatusOK, map[string]string{"ok": action + " done"})
	}
}

func (a *API) handleDockerContainerRemove(w http.ResponseWriter, r *http.Request) {
	s := currentSession(r)
	if a.docker == nil {
		writeErr(w, r, http.StatusServiceUnavailable, "DOCKER_UNAVAILABLE", "docker client not configured", nil)
		return
	}
	id := chi.URLParam(r, "id")
	force := r.URL.Query().Get("force") == "1"
	ctx, cancel := contextWithTimeout(r)
	defer cancel()
	if err := a.docker.RemoveContainer(ctx, id, force); err != nil {
		a.dockerFail(w, r, err)
		return
	}
	a.activity.Log(s.UserID, s.Username, "docker_remove", "container", id, map[string]string{"force": strconv.FormatBool(force)})
	writeData(w, r, http.StatusOK, map[string]string{"ok": "removed"})
}

func (a *API) handleDockerContainerLogs(w http.ResponseWriter, r *http.Request) {
	if a.docker == nil {
		writeErr(w, r, http.StatusServiceUnavailable, "DOCKER_UNAVAILABLE", "docker client not configured", nil)
		return
	}
	id := chi.URLParam(r, "id")
	tail := 200
	if t := r.URL.Query().Get("tail"); t != "" {
		if v, err := strconv.Atoi(t); err == nil && v > 0 && v <= 5000 {
			tail = v
		}
	}
	ctx, cancel := contextWithTimeout(r)
	defer cancel()
	logs, err := a.docker.ContainerLogs(ctx, id, tail)
	if err != nil {
		a.dockerFail(w, r, err)
		return
	}
	writeData(w, r, http.StatusOK, map[string]string{"logs": logs})
}

func (a *API) handleDockerContainerInspect(w http.ResponseWriter, r *http.Request) {
	if a.docker == nil {
		writeErr(w, r, http.StatusServiceUnavailable, "DOCKER_UNAVAILABLE", "docker client not configured", nil)
		return
	}
	id := chi.URLParam(r, "id")
	ctx, cancel := contextWithTimeout(r)
	defer cancel()
	raw, err := a.docker.InspectContainer(ctx, id)
	if err != nil {
		a.dockerFail(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"success": true, "data": raw})
}

func (a *API) handleDockerImages(w http.ResponseWriter, r *http.Request) {
	if a.docker == nil {
		writeErr(w, r, http.StatusServiceUnavailable, "DOCKER_UNAVAILABLE", "docker client not configured", nil)
		return
	}
	ctx, cancel := contextWithTimeout(r)
	defer cancel()
	images, err := a.docker.ListImages(ctx)
	if err != nil {
		a.dockerFail(w, r, err)
		return
	}
	type row struct {
		ID      string `json:"id"`
		Tag     string `json:"tag"`
		Size    int64  `json:"size"`
		Created int64  `json:"created"`
	}
	out := make([]row, 0, len(images))
	for _, img := range images {
		out = append(out, row{ID: trimImageID(img.ID), Tag: img.Tag(), Size: img.Size, Created: img.Created})
	}
	writeData(w, r, http.StatusOK, out)
}

func trimImageID(id string) string {
	if len(id) > 19 && id[:7] == "sha256:" {
		return id[7:19]
	}
	return id
}

func (a *API) handleDockerVolumes(w http.ResponseWriter, r *http.Request) {
	if a.docker == nil {
		writeErr(w, r, http.StatusServiceUnavailable, "DOCKER_UNAVAILABLE", "docker client not configured", nil)
		return
	}
	ctx, cancel := contextWithTimeout(r)
	defer cancel()
	volumes, err := a.docker.ListVolumes(ctx)
	if err != nil {
		a.dockerFail(w, r, err)
		return
	}
	writeData(w, r, http.StatusOK, volumes)
}

func (a *API) handleDockerNetworks(w http.ResponseWriter, r *http.Request) {
	if a.docker == nil {
		writeErr(w, r, http.StatusServiceUnavailable, "DOCKER_UNAVAILABLE", "docker client not configured", nil)
		return
	}
	ctx, cancel := contextWithTimeout(r)
	defer cancel()
	networks, err := a.docker.ListNetworks(ctx)
	if err != nil {
		a.dockerFail(w, r, err)
		return
	}
	writeData(w, r, http.StatusOK, networks)
}
