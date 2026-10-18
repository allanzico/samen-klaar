package workflow

import "errors"

// MaxRequestPhotos bounds the photos a customer can attach to one request.
const MaxRequestPhotos = 6

// RequestPhoto is a customer-supplied picture of the problem. It is stored privately, is never sent to
// an AI provider, and is shown only to the owning customer and the professional assigned to the job.
// It is deliberately separate from completion Evidence, which documents finished work.
type RequestPhoto struct {
	AssetID     string `json:"assetId"`
	ContentType string `json:"contentType"`
	Label       string `json:"label"`
	AddedAt     string `json:"addedAt,omitempty"`
}

func recordRequestPhoto(state *State, input Action) error {
	photo := input.Evidence
	if input.Actor != "customer" || state.Job.Paid || photo.AssetID == "" || (photo.ContentType != "image/jpeg" && photo.ContentType != "image/png" && photo.ContentType != "image/webp") {
		return errors.New("a customer can attach a JPEG, PNG or WebP photo to an open request")
	}
	if len(state.RequestPhotos) >= MaxRequestPhotos {
		return errors.New("a request can have at most six photos")
	}
	for _, existing := range state.RequestPhotos {
		if existing.AssetID == photo.AssetID {
			return errors.New("this photo has already been added")
		}
	}
	state.RequestPhotos = append(state.RequestPhotos, RequestPhoto{AssetID: photo.AssetID, ContentType: photo.ContentType, Label: photo.Label, AddedAt: input.PhotoAddedAt})
	if state.Job.ID != "" {
		addEvent(&state.Job, "Customer added a photo of the problem", "done")
	}
	return nil
}
