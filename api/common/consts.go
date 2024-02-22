package common

const (
	NamespaceLabel = "_namespace"
	K8sNameLabel   = "_k8sname"
	ClusterIDLabel = "_clusterid"

	Created        = "Created"
	Updated        = "Updated"
	Deleted        = "Deleted"
	Provisioned    = "Provisioned"
	NotProvisioned = "NotProvisioned"

	CreateInProgress = "CreateInProgress"
	UpdateInProgress = "UpdateInProgress"
	DeleteInProgress = "DeleteInProgress"
	InProgress       = "InProgress"
	Finished         = "Finished"

	CreateFailed      = "CreateFailed"
	UpdateFailed      = "UpdateFailed"
	DeleteFailed      = "DeleteFailed"
	Failed            = "Failed"
	ShareFailed       = "ShareFailed"
	ShareSucceeded    = "ShareSucceeded"
	ShareNotSupported = "ShareNotSupported"
	UnShareFailed     = "UnShareFailed"
	UnShareSucceeded  = "UnShareSucceeded"

	Blocked = "Blocked"
	Unknown = "Unknown"

	// Cred Rotation
	CredPreparing = "Preparing"
	CredRotating  = "Rotating"
)
