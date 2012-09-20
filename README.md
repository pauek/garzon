
1. Create directory ('VMs'), change into it.

  $ mkdir VMs
  $ cd VMs

2. Download TinyCoreLinux Files

  $ site=http://distro.ibiblio.org/tinycorelinux/4.x/x86/release/distribution_files/
  $ wget $site/core.gz
  $ wget $site/vmlinuz

3. Remaster core.gz into initrd.gz

  $ grz-vm remaster core.gz initrd.gz
 
4. Create disk image

  $ grz-vm createimage garzon.img

5. Install 'packages'

  $ grz-vm pkglist
  gcc
  go
  ...
  $ grz-vm install garzon.img gcc go ...

6. Launch

  $ grz-vm launch garzon.img
