/*
 * Copyright (c) 2021 The GoPlus Authors (goplus.org). All rights reserved.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package modfetch

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/goplus/mod/env"
	"github.com/goplus/mod/modcache"
	"github.com/goplus/mod/modload"
	"golang.org/x/mod/module"
	"golang.org/x/mod/semver"
)

// -----------------------------------------------------------------------------

// Get downloads a modPath to GOMODCACHE.
//
func Get(env *env.Gop, modPath string, noCache ...bool) (mod module.Version, isClass bool, err error) {
	if noCache == nil || !noCache[0] {
		mod, isClass, err = getFromCache(modPath, env)
		if err != syscall.ENOENT {
			return
		}
	}
	var stdout, stderr bytes.Buffer
	var modPathVer = modPath
	if strings.IndexByte(modPath, '@') < 0 {
		modPathVer += "@latest"
	}
	cmd := exec.Command("go", "install", modPathVer)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	cmd.Run()
	if stderr.Len() > 0 {
		mod, isClass, err = getResult(stderr.String(), env)
		if err != syscall.ENOENT {
			return
		}
	}
	return getFromCache(modPath, env)
}

func getResult(data string, env *env.Gop) (mod module.Version, isClass bool, err error) {
	// go: downloading github.com/xushiwei/foogop v0.1.0
	const downloading = "go: downloading "
	if strings.HasPrefix(data, downloading) {
		if pos := strings.IndexByte(data, '\n'); pos > 0 {
			fmt.Fprintln(os.Stderr, "gop:", data[4:pos])
		}
		return tryConvGoMod(data[len(downloading):], &data, env)
	}
	err = syscall.ENOENT
	return
}

func tryConvGoMod(data string, next *string, env *env.Gop) (mod module.Version, isClass bool, err error) {
	err = syscall.ENOENT
	if pos := strings.IndexByte(data, '\n'); pos > 0 {
		line := data[:pos]
		*next = data[pos+1:]
		if pos = strings.IndexByte(line, ' '); pos > 0 {
			mod.Path, mod.Version = line[:pos], line[pos+1:]
			if dir, e := modcache.Path(mod); e == nil {
				isClass, err = convGoMod(dir, env)
			}
		}
	}
	return
}

func convGoMod(dir string, env *env.Gop) (isClass bool, err error) {
	if env == nil {
		return
	}
	mod, err := modload.Load(dir, 0)
	if err != nil {
		return
	}
	os.Chmod(dir, 0755)
	defer os.Chmod(dir, 0555)
	return mod.Classfile != nil, mod.UpdateGoMod(env, true)
}

// -----------------------------------------------------------------------------

func getFromCache(modPath string, env *env.Gop) (modVer module.Version, isClass bool, err error) {
	modRoot, modVer, err := lookupFromCache(modPath)
	if err != nil {
		return
	}
	isClass, err = convGoMod(modRoot, env)
	return
}

func lookupFromCache(modPath string) (modRoot string, mod module.Version, err error) {
	mod.Path = modPath
	pos := strings.IndexByte(modPath, '@')
	if pos > 0 {
		mod.Path, mod.Version = modPath[:pos], modPath[pos+1:]
	}
	encPath, err := module.EscapePath(mod.Path)
	if err != nil {
		return
	}
	modRoot = filepath.Join(modcache.GOMODCACHE, encPath+"@"+mod.Version)
	if pos > 0 { // has version
		fi, e := os.Stat(modRoot)
		if e != nil || !fi.IsDir() {
			err = syscall.ENOENT
		}
		return
	}
	dir, fname := filepath.Split(modRoot)
	fis, err := os.ReadDir(dir)
	if err != nil {
		err = errors.Unwrap(err)
		return
	}
	err = syscall.ENOENT
	for _, fi := range fis {
		if fi.IsDir() {
			if name := fi.Name(); strings.HasPrefix(name, fname) {
				ver := name[len(fname):]
				if semver.Compare(mod.Version, ver) < 0 {
					modRoot, mod.Version, err = dir+name, ver, nil
				}
			}
		}
	}
	return
}

// -----------------------------------------------------------------------------
