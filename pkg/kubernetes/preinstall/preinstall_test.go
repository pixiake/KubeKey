package preinstall

import (
	"testing"
)

func Test_sha256sum(t *testing.T) {
	for path, expect := range map[string]string{
		"testdata/hello.txt":  "09ca7e4eaa6e8ae9c7d261167129184883644d07dfba7cbfbc4c8a2e08360d5b",
		"testdata/goodby.txt": "3270412cf43cacfe2945bd88187b24feaf541633a0aa6225435b8ab855f0ea33",
	} {
		sum, err := sha256sum(path)
		if err != nil {
			t.Errorf("sha256sum failure: %s", err)
		}
		if sum != expect {
			t.Errorf("sha256sum %s want sum: %s but got:%s", path, expect, sum)
		}
	}
}
