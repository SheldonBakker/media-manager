package api

// settingOn reads a "1"/"0" switch from the settings; unset (or unreadable)
// gives defaultOn.
func (s *Server) settingOn(key string, defaultOn bool) bool {
	v, err := s.Settings.Get(key)
	if err != nil || v == "" {
		return defaultOn
	}
	return v == "1"
}

// setBool saves a switch as "1" or "0".
func setBool(st settingSetter, key string, on bool) error {
	v := "0"
	if on {
		v = "1"
	}
	return st.Set(key, v, false)
}
