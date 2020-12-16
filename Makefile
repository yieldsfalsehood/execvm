
execvm:
	go build -i

data.tar.gz:
	tar -c execvm | abuild-tar --hash | gzip -9 > data.tar.gz

control.tar.gz:
	tar -c .PKGINFO | abuild-tar --cut | gzip -9 > control.tar.gz
