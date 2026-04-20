# CGO off so the binary is statically linked and works on any distro
# that ships no matching glibc; matches the Lynx release binaries'
# build flags.
.PHONY: build clean

build:
	CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o go-compiled ./main.go

clean:
	rm -f go-compiled
