// MDM stewardship resolution: pending-stewardship records can be promoted
// to golden, merged into an existing golden record, or rejected — the
// governance loop behind the "pending stewardship" story.
package api

import (
	"net/http"

	"github.com/santhoshmp/flowforge/internal/models"
)

func (s *Server) registerMDMResolve() {
	s.mux.HandleFunc("POST /api/v1/mdm/{entity}/resolve", s.resolveMDMRecord)
}

// resolveMDMRecord: POST /api/v1/mdm/{entity}/resolve
// {"id": "V-1012", "action": "promote"|"merge"|"reject", "mergeInto": "V-1001"}
func (s *Server) resolveMDMRecord(w http.ResponseWriter, r *http.Request) {
	entityKey := r.PathValue("entity")
	var req struct {
		ID        string `json:"id"`
		Action    string `json:"action"`
		MergeInto string `json:"mergeInto"`
	}
	if err := decode(r, &req); err != nil {
		writeErr(w, 400, err.Error())
		return
	}
	if req.ID == "" {
		writeErr(w, 400, "id is required")
		return
	}

	entities, err := s.store.ListMDM()
	if err != nil {
		writeErr(w, 500, err.Error())
		return
	}
	var entity *models.MDMEntity
	for i := range entities {
		if entities[i].Key == entityKey {
			entity = &entities[i]
			break
		}
	}
	if entity == nil {
		writeErr(w, 404, "entity not found")
		return
	}

	idx := -1
	for i, rec := range entity.Records {
		if rec["id"] == req.ID {
			idx = i
			break
		}
	}
	if idx == -1 {
		writeErr(w, 404, "record not found")
		return
	}
	if entity.Records[idx]["status"] != "pending stewardship" {
		writeErr(w, 400, "only pending-stewardship records can be resolved")
		return
	}

	record := entity.Records[idx]
	switch req.Action {
	case "promote":
		record["status"] = "golden"
		entity.Records[idx] = record
		if err := s.store.UpsertMDM(*entity); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		s.audit("You", "MDM record promoted", entity.Label+" · "+req.ID+" — pending stewardship → golden", "mdm")
		writeJSON(w, 200, entity)
	case "merge":
		if req.MergeInto == "" {
			writeErr(w, 400, "mergeInto is required for merge")
			return
		}
		target := -1
		for i, rec := range entity.Records {
			if rec["id"] == req.MergeInto && rec["status"] == "golden" {
				target = i
				break
			}
		}
		if target == -1 {
			writeErr(w, 400, "mergeInto must name an existing golden record")
			return
		}
		entity.Records = append(entity.Records[:idx], entity.Records[idx+1:]...)
		if err := s.store.UpsertMDM(*entity); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		s.audit("You", "MDM records merged", entity.Label+" · "+req.ID+" merged into "+req.MergeInto, "mdm")
		writeJSON(w, 200, entity)
	case "reject":
		entity.Records = append(entity.Records[:idx], entity.Records[idx+1:]...)
		if err := s.store.UpsertMDM(*entity); err != nil {
			writeErr(w, 500, err.Error())
			return
		}
		s.audit("You", "MDM record rejected", entity.Label+" · "+req.ID+" — removed from pending stewardship", "mdm")
		writeJSON(w, 200, entity)
	default:
		writeErr(w, 400, `action must be "promote", "merge", or "reject"`)
	}
}
