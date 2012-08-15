#!/bin/bash

kvm -kernel VM/vmlinuz \
    -initrd VM/initrd.gz \
    -append "tce=vda kmap=qwerty/es vga=788 nodhcp" \
    -drive file=VM/tce.img,if=virtio \
    -serial stdio \
    -net none \
    $*
