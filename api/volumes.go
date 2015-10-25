package api

import "github.com/libopenstorage/openstorage/proto/openstorage"

// OptionKey specifies a set of recognized query params
type OptionKey string

const (
	// OptName query parameter used to lookup volume by name
	OptName = OptionKey("Name")
	// OptVolumeID query parameter used to lookup volume by ID.
	OptVolumeID = OptionKey("VolumeID")
	// OptLabel query parameter used to lookup volume by set of labels.
	OptLabel = OptionKey("Label")
	// OptConfigLabel query parameter used to lookup volume by set of labels.
	OptConfigLabel = OptionKey("ConfigLabel")
)

// VolumeActionParam desired action on volume
type VolumeActionParam int

const (
	// ParamIgnore user should ignore the value of the parameter.
	ParamIgnore VolumeActionParam = iota
	// ParamOff maps to the boolean value false.
	ParamOff
	// ParamOn maps to the boolean value true.
	ParamOn
)

// VolumeStateAction is the body of the REST request to specify desired actions
type VolumeStateAction struct {
	// Attach or Detach volume
	Attach VolumeActionParam `json:"attach"`
	// Mount or unmount volume
	Mount VolumeActionParam `json:"mount"`
	// MountPath
	MountPath string `json:"mount_path"`
	// DevicePath returned in Attach
	DevicePath string `json:"device_path"`
}

// VolumeStateResponse is the body of the REST response
type VolumeStateResponse struct {
	// VolumeStateRequest the current state of the volume
	VolumeStateAction
	VolumeResponse
}

// VolumeResponse is embedded in all REST responses.
type VolumeResponse struct {
	// Error is "" on success or contains the error message on failure.
	Error string `json:"error"`
}

// SnapCreateRequest request body to create a snap.
type SnapCreateRequest struct {
	ID       string                     `json:"id"`
	Locator  *openstorage.VolumeLocator `json:"locator"`
	Readonly bool                       `json:"readonly"`
}

// SnapCreateResponse response body to SnapCreateRequest
type SnapCreateResponse struct {
	*openstorage.VolumeCreateResponse
}

// ResponseStatusNew create VolumeResponse from error
func ResponseStatusNew(err error) VolumeResponse {
	if err == nil {
		return VolumeResponse{}
	}
	return VolumeResponse{Error: err.Error()}
}
