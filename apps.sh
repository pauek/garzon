#!/bin/bash

# Prepare an image of apps for Tiny Core Linux VM
# 

mirror="http://l4u-00.jinr.ru/LinuxArchive/Ftp/tinycorelinux/"
dir="4.x/x86/tcz/"
x86_64=''

imgname=$1
if [ -z $1 ]; then
    imgname='tce'
fi

function apps_image() {
    case $1 in
    create)
       dd if=/dev/zero of=${imgname}.img bs=1M count=500 # 500Mb enough?
       sudo mkfs.ext3 -F ${imgname}.img ;;
    mount)
       mkdir ${imgname}
       sudo mount -o loop ${imgname}.img tce ;;
    umount)
       sudo umount ${imgname}
       rmdir ${imgname}
    esac
}

function add_packages() {
    list=""
    case $1 in 
        gcc)
            list="gcc gcc_libs binutils gmp mpfr eglibc_base-dev libmpc" ;;
    esac
    tcedir=${imgname}/tce/optional
    mkdir -p ${tcedir}
    pushd ${tcedir}
    for file in $list; do
        wget ${mirror}${dir}${file}.tcz
    done
    popd
}

# Do it
apps_image create
apps_image mount
add_packages gcc
apps_image umount