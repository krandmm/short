package types

import (
	"encoding/json"
	"strings"

	"k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/koki/short/util"
)

type PersistentVolumeWrapper struct {
	PersistentVolume PersistentVolume `json:"persistent_volume"`
}

type PersistentVolume struct {
	PersistentVolumeMeta
	PersistentVolumeSource
}

type PersistentVolumeMeta struct {
	Version     string            `json:"version,omitempty"`
	Cluster     string            `json:"cluster,omitempty"`
	Name        string            `json:"name,omitempty"`
	Namespace   string            `json:"namespace,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`

	Storage       *resource.Quantity            `json:"storage,omitempty"`
	AccessModes   *AccessModes                  `json:"modes,omitempty"`
	Claim         *v1.ObjectReference           `json:"claim,omitempty"`
	ReclaimPolicy PersistentVolumeReclaimPolicy `json:"reclaim,omitempty"`
	StorageClass  string                        `json:"storage_class,omitempty"`

	// comma-separated list of options
	MountOptions string `json:"mount_options,omitempty" protobuf:"bytes,7,opt,name=mountOptions"`

	Status *v1.PersistentVolumeStatus `json:"status,omitempty"`
}

type PersistentVolumeReclaimPolicy string

const (
	PersistentVolumeReclaimRecycle PersistentVolumeReclaimPolicy = "recycle"
	PersistentVolumeReclaimDelete  PersistentVolumeReclaimPolicy = "delete"
	PersistentVolumeReclaimRetain  PersistentVolumeReclaimPolicy = "retain"
)

type PersistentVolumeSource struct {
	GcePD    *GcePDVolume
	AwsEBS   *AwsEBSVolume
	HostPath *HostPathVolume
}

// comma-separated list of modes
type AccessModes struct {
	Modes []v1.PersistentVolumeAccessMode
}

func (a *AccessModes) ToString() (string, error) {
	if a == nil {
		return "", nil
	}

	if len(a.Modes) == 0 {
		return "", nil
	}

	modes := make([]string, len(a.Modes))
	for i, mode := range a.Modes {
		switch mode {
		case v1.ReadOnlyMany:
			modes[i] = "ro"
		case v1.ReadWriteMany:
			modes[i] = "rw"
		case v1.ReadWriteOnce:
			modes[i] = "rw-once"
		default:
			return "", util.InvalidInstanceError(mode)
		}
	}

	return strings.Join(modes, ","), nil
}

func (a *AccessModes) InitFromString(s string) error {
	modes := strings.Split(s, ",")
	if len(modes) == 0 {
		a.Modes = nil
		return nil
	}

	a.Modes = make([]v1.PersistentVolumeAccessMode, len(modes))
	for i, mode := range modes {
		switch mode {
		case "ro":
			a.Modes[i] = v1.ReadOnlyMany
		case "rw":
			a.Modes[i] = v1.ReadWriteMany
		case "rw-once":
			a.Modes[i] = v1.ReadWriteOnce
		default:
			return util.InvalidValueErrorf(a, "couldn't parse (%s)", s)
		}
	}

	return nil
}

func (a AccessModes) MarshalJSON() ([]byte, error) {
	str, err := a.ToString()
	if err != nil {
		return nil, err
	}

	b, err := json.Marshal(&str)
	if err != nil {
		return nil, util.InvalidInstanceErrorf(a, "couldn't marshal to JSON: %s", err.Error())
	}

	return b, nil
}

func (a *AccessModes) UnmarshalJSON(data []byte) error {
	str := ""
	err := json.Unmarshal(data, &str)
	if err != nil {
		return util.InvalidValueForTypeErrorf(string(data), a, "couldn't unmarshal from JSON: %s", err.Error())
	}

	return a.InitFromString(str)
}

func (v *PersistentVolume) UnmarshalJSON(data []byte) error {
	err := json.Unmarshal(data, &v.PersistentVolumeSource)
	if err != nil {
		return util.InvalidValueForTypeErrorf(string(data), v, "couldn't unmarshal volume source from JSON: %s", err.Error())
	}

	err = json.Unmarshal(data, &v.PersistentVolumeMeta)
	if err != nil {
		return util.InvalidValueForTypeErrorf(string(data), v, "couldn't unmarshal metadata from JSON: %s", err.Error())
	}

	return nil
}

func (v PersistentVolume) MarshalJSON() ([]byte, error) {
	b, err := json.Marshal(v.PersistentVolumeMeta)
	if err != nil {
		return nil, util.InvalidInstanceErrorf(v, "couldn't marshal metadata to JSON: %s", err.Error())
	}

	bb, err := json.Marshal(v.PersistentVolumeSource)
	if err != nil {
		return nil, err
	}

	metaObj := map[string]interface{}{}
	err = json.Unmarshal(b, &metaObj)
	if err != nil {
		return nil, util.InvalidValueForTypeErrorf(string(b), v.PersistentVolumeMeta, "couldn't convert metadata to dictionary: %s", err.Error())
	}

	sourceObj := map[string]interface{}{}
	err = json.Unmarshal(bb, &sourceObj)
	if err != nil {
		return nil, util.InvalidValueForTypeErrorf(string(bb), v.PersistentVolumeSource, "couldn't convert volume source to dictionary: %s", err.Error())
	}

	// Merge metadata with volume-source
	for key, val := range metaObj {
		sourceObj[key] = val
	}

	result, err := json.Marshal(sourceObj)
	if err != nil {
		return nil, util.InvalidValueForTypeErrorf(result, v, "couldn't marshal PersistentVolume-as-dictionary to JSON: %s", err.Error())
	}

	return result, nil
}

func (v *PersistentVolumeSource) UnmarshalJSON(data []byte) error {
	var err error
	obj := map[string]interface{}{}
	err = json.Unmarshal(data, &obj)
	if err != nil {
		return util.InvalidValueErrorf(string(data), "expected dictionary for persistent volume")
	}

	var selector []string
	if val, ok := obj["vol_id"]; ok {
		if volName, ok := val.(string); ok {
			selector = strings.Split(volName, ":")
		} else {
			return util.InvalidValueErrorf(string(data), "expected string for key \"vol_id\"")
		}
	}

	volType, err := util.GetStringEntry(obj, "vol_type")
	if err != nil {
		return err
	}

	return v.Unmarshal(obj, volType, selector)
}

func (v *PersistentVolumeSource) Unmarshal(obj map[string]interface{}, volType string, selector []string) error {
	switch volType {
	case VolumeTypeGcePD:
		v.GcePD = &GcePDVolume{}
		return v.GcePD.Unmarshal(obj, selector)
	case VolumeTypeAwsEBS:
		v.AwsEBS = &AwsEBSVolume{}
		return v.AwsEBS.Unmarshal(obj, selector)
	case VolumeTypeHostPath:
		v.HostPath = &HostPathVolume{}
		return v.HostPath.Unmarshal(selector)
	default:
		return util.InvalidValueErrorf(volType, "unsupported volume type (%s)", volType)
	}
}

func (v PersistentVolumeSource) MarshalJSON() ([]byte, error) {
	var marshalledVolume *MarshalledVolume
	var err error
	if v.GcePD != nil {
		marshalledVolume, err = v.GcePD.Marshal()
	}
	if v.AwsEBS != nil {
		marshalledVolume, err = v.AwsEBS.Marshal()
	}
	if v.HostPath != nil {
		marshalledVolume, err = v.HostPath.Marshal()
	}

	if err != nil {
		return nil, err
	}

	if marshalledVolume == nil {
		return nil, util.InvalidInstanceErrorf(v, "empty volume definition")
	}

	if len(marshalledVolume.ExtraFields) == 0 {
		marshalledVolume.ExtraFields = map[string]interface{}{}
	}

	obj := marshalledVolume.ExtraFields
	obj["vol_type"] = marshalledVolume.Type
	if len(marshalledVolume.Selector) > 0 {
		obj["vol_id"] = strings.Join(marshalledVolume.Selector, ":")
	}

	return json.Marshal(obj)
}
