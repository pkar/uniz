//go:build !darwin && !linux

package fileops

func fillSys(*StatInfo) {}
