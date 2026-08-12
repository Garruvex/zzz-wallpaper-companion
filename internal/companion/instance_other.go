//go:build !windows

package companion

func acquireSingleInstance() (func(), bool, error) {
	return func() {}, false, nil
}
