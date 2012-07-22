#!/bin/bash

kvm -kernel vmlinuz \
    -initrd initrd.gz \
    -append "tce=vda kmap=qwerty/es vga=788 nodhcp" \
    -drive file=tce.img,if=virtio \
    -serial stdio \
    -net none \
    $*
