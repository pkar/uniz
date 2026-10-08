package sysinfo

import "syscall"

func cstring[T int8 | uint8](a []T) string {
	b := make([]byte, 0, len(a))
	for _, c := range a {
		if c == 0 {
			break
		}
		b = append(b, byte(c))
	}
	return string(b)
}

// Uname returns the kernel's identification.
func Uname() (Info, error) {
	var u syscall.Utsname
	if err := syscall.Uname(&u); err != nil {
		return Info{}, err
	}
	return Info{
		Sysname:  cstring(u.Sysname[:]),
		Nodename: cstring(u.Nodename[:]),
		Release:  cstring(u.Release[:]),
		Version:  cstring(u.Version[:]),
		Machine:  cstring(u.Machine[:]),
	}, nil
}
