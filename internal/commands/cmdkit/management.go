package cmdkit

// Management API facts shared by a core group and an add-on (deploy's
// post-deploy watch reads indexer health and the log viewer).

// IndexerHealthStatus reads an Examine index's health from the
// /indexer/{name} payload.
func IndexerHealthStatus(payload any) string {
	object, ok := payload.(map[string]any)
	if !ok {
		return ""
	}
	health, ok := object["healthStatus"].(map[string]any)
	if !ok {
		// Pre-16 servers returned healthStatus as a plain string.
		if status, ok := object["healthStatus"].(string); ok {
			return status
		}
		return ""
	}
	status, _ := health["status"].(string)
	return status
}

// LogViewerLogPath is the modern log-viewer entries route.
const LogViewerLogPath = "/log-viewer/log"
