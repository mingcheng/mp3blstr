//go:build darwin

package filemanager

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"github.com/ebitengine/purego"
	"github.com/ebitengine/purego/objc"
)

var loadFoundation = sync.OnceValue(func() error {
	_, err := purego.Dlopen("/System/Library/Frameworks/Foundation.framework/Foundation", purego.RTLD_NOW)
	return err
})

// Trash uses the system API, including the correct trash on external volumes.
// It never falls back to permanently deleting a file.
func Trash(path string) error {
	if path == "" || strings.ContainsRune(path, 0) {
		return fmt.Errorf("invalid Trash path %q", path)
	}
	path, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	if err := loadFoundation(); err != nil {
		return fmt.Errorf("loading Trash service: %w", err)
	}
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	pool := objc.ID(objc.GetClass("NSAutoreleasePool")).Send(objc.RegisterName("new"))
	defer pool.Send(objc.RegisterName("drain"))
	text := objc.ID(objc.GetClass("NSString")).Send(objc.RegisterName("stringWithUTF8String:"), path)
	url := objc.ID(objc.GetClass("NSURL")).Send(objc.RegisterName("fileURLWithPath:"), text)
	manager := objc.ID(objc.GetClass("NSFileManager")).Send(objc.RegisterName("defaultManager"))
	var nativeError objc.ID
	ok := objc.Send[bool](manager, objc.RegisterName("trashItemAtURL:resultingItemURL:error:"), url, uintptr(0), &nativeError)
	if !ok {
		description := "system Trash operation failed"
		if nativeError != 0 {
			s := nativeError.Send(objc.RegisterName("localizedDescription"))
			description = objc.Send[string](s, objc.RegisterName("UTF8String"))
		}
		return fmt.Errorf("moving %q to Trash: %s", path, description)
	}
	return nil
}
