package common

const (
	ManagedByBTPOperatorLabel = "services.cloud.sap.com/managed-by-sap-btp-operator"
	ClusterSecretLabel        = "services.cloud.sap.com/cluster-secret"
	InstanceSecretRefLabel    = "services.cloud.sap.com/secret-ref_"
	WatchSecretAnnotation     = "services.cloud.sap.com/watch-secret-"

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

	// Constance for seceret template
	InstanceKey    = "instance"
	CredentialsKey = "credentials"
)
