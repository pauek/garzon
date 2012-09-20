
Creating a Virtual Machine for Garzon
-------------------------------------

1. Create directory ('VMs'), change into it.

  $ mkdir VMs
  $ cd VMs

2. Download TinyCoreLinux Files

  $ grz-vm download

3. Remaster core.gz into initrd.gz

  $ grz-vm remaster core.gz initrd.gz
 
4. Create disk image

  $ grz-vm createimg my.img

5. Install 'packages'

  $ grz-vm pkglist
  gcc
  go
  ...
  $ grz-vm install my.img gcc go ...

6. Convert image to qcow2 (enable snapshots)
 
  $ grz-vm convert my.img my.qcow2

7. Launch

  $ grz-vm launch my.qcow2
  $ grz-vm launch my.qcow2 -nographic
