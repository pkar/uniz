//go:build darwin || freebsd || netbsd || openbsd || dragonfly

package sysinfo

import (
	"errors"
	"syscall"
)

// Uname returns the kernel's identification using sysctl.
func Uname() (Info, error) {
	var info Info
	var errs []error
	for name, dst := range map[string]*string{
		"kern.ostype":    &info.Sysname,
		"kern.hostname":  &info.Nodename,
		"kern.osrelease": &info.Release,
		"kern.version":   &info.Version,
		"hw.machine":     &info.Machine,
	} {
		v, err := syscall.Sysctl(name)
		if err != nil {
			errs = append(errs, err)
		}
		*dst = v
	}
	return info, errors.Join(errs...)
}
