package types

const AddLabelValuesOperation = "add_values"

type Labels map[string][]string

type LabelChange struct {
	Operation string   `json:"op"`
	Key       string   `json:"key"`
	Values    []string `json:"values"`
}

type OperationCategory string

const (
	CREATE  OperationCategory = "create"
	UPDATE  OperationCategory = "update"
	DELETE  OperationCategory = "delete"
	SHARE   OperationCategory = "share"
	UNSHARE OperationCategory = "un-share"
)

type OperationState string

const (
	PENDING    OperationState = "pending"
	SUCCEEDED  OperationState = "succeeded"
	INPROGRESS OperationState = "in progress"
	FAILED     OperationState = "failed"
)
