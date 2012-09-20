
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

6. Launch

  $ grz-vm launch my.img
  $ grz-vm launch my.img -nographic
