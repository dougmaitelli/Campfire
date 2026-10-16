package model

// BuiltinEnvironment returns the reserved Ember environment names.
func BuiltinEnvironment() map[string]string {
	return map[string]string{
		"SERVER_PORT": "", "SERVER_IP": "", "SERVER_MEMORY": "",
		"SERVER_CPU": "", "SERVER_THREADS": "", "SERVER_DISK": "", "SERVER_SWAP": "",
	}
}
