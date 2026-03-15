# SPDX-License-Identifier: GPL-3.0-or-later
# MiniKernel — UPC Level 5 kernel skeleton

.PHONY: build boot clean

build:
	go run build.go

boot: build
	wotan-ctl boot --kernel minikernel.bin --args "console=tty0"

clean:
	rm -f minikernel.bin
