package common

// Reserved system field names shared by every document model. They live at the
// lowest level of the dependency graph (common) so that core/sanitize can
// recognize and preserve them without importing core/data.
const (
	DocumentIDField   = "_id_"
	MetadataField     = "_metadata_"
	MetadataChecksum  = "checksum"
	MetadataSignature = "signature"
	MetadataVersion   = "version"
	MetadataCreated   = "created"
	MetadataUpdated   = "updated"
)