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
tce=${imgname}/tce

function download() {
    pushd ${tce}/optional
    for file in $*; do
        if ! [ -f ${file}.tcz ]; then
            sudo wget ${mirror}${dir}${file}.tcz
        fi
        if ! [ -f ${file}.tcz.dep ]; then
            sudo wget ${mirror}${dir}${file}.tcz.dep
        fi
    done
    popd
}

function purge() {
    pushd ${tce}/optional
    for file in $*; do
        sudo rm -f ${file}.tcz{,.dep}
    done
    popd
}

function onboot() {
    for file in $*; do
        echo $file | sudo sh -c "cat >> ${tce}/onboot.lst"
    done
}

function image() {
    for cmd in $*; do
        case $cmd in
            create)
                dd if=/dev/zero of=${imgname}.img bs=1M count=500 # 500Mb enough?
                sudo mkfs.ext3 -F ${imgname}.img ;;
            mount)
                mkdir ${imgname}
                sync
                sudo mount -o loop ${imgname}.img ${imgname} ;;
            umount)
                sync
                sudo umount ${imgname}
                rmdir ${imgname} 
                sync ;;
            init)
                sudo mkdir -p ${tce}/{optional,ondemand}
                sudo touch ${tce}/onboot.lst
                sudo touch ${tce}/bootlocal.sh # this is called from /opt/bootlocal.sh
                download mirrors kmaps
                onboot mirrors.tcz kmaps.tcz ;;
        esac
    done
}


function add_go() {
    image mount 

    # Decompress source
    sudo mkdir -p ${imgname}/src
    pushd ${imgname}/src
    sudo wget http://go.googlecode.com/files/go1.0.2.src.tar.gz
    sudo tar xzvf go1.0.2.src.tar.gz
    popd

    # Install bash + gcc (+ linux headers)
    download bash ncurses ncurses-common \
        gcc gcc_libs binutils gmp mpfr eglibc_base-dev libmpc \
        linux-3.0.1_api_headers

    image umount

    # Compile Go inside the VM
    (sleep 10; cat <<EOF
cd /mnt/vda/src/go/src
su tc -c "tce-load -i bash"
su tc -c "tce-load -i gcc"
su tc -c "tce-load -i eglibc_base-dev.tcz"
su tc -c "tce-load -i linux-3.0.1_api_headers"
export PATH=/usr/local/bin:\$PATH
bash ./make.bash
poweroff
EOF
    ) | ./launch.sh ${imgname}.img -serial stdio -m 512M

    # Set environment + erase unneeded packages
    image mount
    sudo sh -c "cat >> ${imgname}/tce/bootlocal.sh" <<EOF
cat >> /etc/profile.d/go.sh <<INNEREOF
# Go Environment
export GOROOT=/mnt/vda/src/go
export PATH=\\\$PATH:\\\$GOROOT/bin
INNEREOF
EOF
    purge bash ncurses ncurses-common linux-3.0.1_api_headers
    # purge gcc gcc_libs binutils gmp mpfr eglibc_base-dev libmpc ??
    image umount
}

function add() {
    for pkg in $*; do
        list=""
        case $pkg in 
            gcc)
                image mount
                download gcc gcc_libs binutils gmp mpfr eglibc_base-dev libmpc 
                sudo sh -c "echo eglibc_base-dev.tcz >> ${tce}/optional/gcc.tcz.dep"
                onboot gcc.tcz 
                image umount ;;
            go)
                add_go ;;
        esac
    done
}

image create mount init umount
add go gcc
