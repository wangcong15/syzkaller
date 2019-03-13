// Copyright 2019 syzkaller project authors. All rights reserved.
// Use of this source code is governed by Apache 2 LICENSE that can be found in the LICENSE file.

package build

import (
	"fmt"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/google/syzkaller/pkg/osutil"
)

type freebsd struct{}

func (ctx freebsd) build(targetArch, vmType, kernelDir, outputDir, compiler, userspaceDir,
	cmdlineFile, sysctlFile string, config []byte) error {
	confFile := fmt.Sprintf("%v/sys/%v/conf/SYZKALLER", kernelDir, targetArch)

	if config == nil {
		config = []byte(`
include "./GENERIC"

ident		SYZKALLER
options 	COVERAGE
options 	KCOV
`)
	}
	if err := osutil.WriteFile(confFile, config); err != nil {
		return err
	}

	objDir := filepath.Join(outputDir, "obj")
	if err := ctx.make(kernelDir, objDir, "kernel-toolchain"); err != nil {
		return err
	}
	if err := ctx.make(kernelDir, objDir, "buildkernel", "KERNCONF=SYZKALLER"); err != nil {
		return err
	}

	for _, s := range []struct{ dir, src, dst string }{
		{userspaceDir, "image", "image"},
		{userspaceDir, "key", "key"},
	} {
		fullSrc := filepath.Join(s.dir, s.src)
		fullDst := filepath.Join(outputDir, s.dst)
		if err := osutil.CopyFile(fullSrc, fullDst); err != nil {
			return fmt.Errorf("failed to copy %v -> %v: %v", fullSrc, fullDst, err)
		}
	}

	script := fmt.Sprintf(`
set -eux
md=$(sudo mdconfig -a -t vnode image)
tmpdir=$(mktemp -d)
sudo mount /dev/${md}p3 $tmpdir

sudo MAKEOBJDIRPREFIX=$(pwd)/obj make -C %s installkernel KERNCONF=SYZKALLER DESTDIR=$tmpdir

sudo umount $tmpdir
sudo mdconfig -d -u ${md#md}
`, kernelDir)

	if debugOut, err := osutil.RunCmd(10*time.Minute, outputDir, "/bin/sh", "-c", script); err != nil {
		return fmt.Errorf("error copying kernel: %v\n%v", err, debugOut)
	}
	return nil
}

func (ctx freebsd) clean(kernelDir, targetArch string) error {
	// Builds are non-incremental for now, so we don't need to do anything here.
	return nil
}

func (ctx freebsd) make(kernelDir string, objDir string, makeArgs ...string) error {
	args := append([]string{
		fmt.Sprintf("MAKEOBJDIRPREFIX=%v", objDir),
		"make",
		"-C", kernelDir,
		"-j", strconv.Itoa(runtime.NumCPU()),
	}, makeArgs...)
	_, err := osutil.RunCmd(3*time.Hour, kernelDir, "sh", "-c", strings.Join(args, " "))
	return err
}