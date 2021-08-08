# scp
    import "github.com/tmc/scp"

Package scp provides a simple interface to copying files over a
go.crypto/ssh session.

## func Copy
``` go
func Copy(size int64, mode os.FileMode, fileName string, contents io.Reader, destinationPath string, session *ssh.Session) error
```

## func CopyPath
``` go
func CopyPath(filePath, destinationPath string, session *ssh.Session) error
```
