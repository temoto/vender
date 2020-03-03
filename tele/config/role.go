package tele_config

type Role string

const (
	RoleInvalid Role = ""
	RoleAll     Role = "_all"
	RoleAdmin   Role = "admin"
	RoleControl Role = "control"
	RoleMonitor Role = "monitor"
	RoleVender  Role = "vender"
)

// [a, b], b -> true
// [a, b], c -> false
// [_all], * -> true
func RoleListAllows(allows []string, client Role) bool {
	for _, a := range allows {
		if a == string(RoleAll) || a == string(client) {
			return true
		}
	}
	return false
}
