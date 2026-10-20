package mocksys

// FileExists answers the startup page's "does this program still exist"
// check. The simulated machine lists programs by their Windows paths
// (C:\Program Files\...), which have no counterpart on the host's file
// system, so every listed program counts as installed.
func (n *Native) FileExists(string) bool { return true }
