package imports

import "os"

func openCSV(p string) (*os.File, error) { return os.Open(p) }
