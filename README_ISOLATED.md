# Support for Qemu/KVM containers


The conventional way of running docker containers on Linux leveraging namespaces and cgroups (runc) works great and we absolutely love it! But there are situations where we would like to have a stronger isolation between the container and the host kernel.


The basic idea here is to isolate the running container from the host kernel by running it inside a virtual machine. Traditional hypervisors are good at having a separation between the `guest` kernel and `host` kernel. Our choice of hypervisor is Qemu/KVM.


### Sample Usage


The User need to add `--isolation=qemu` flag to existing docker commands.


```sh
$
$docker run --isolation=qemu  busybox /bin/ls
bin   dev   etc   home  proc  root  sys   tmp   usr   var
$
```
With `--isolation=qemu` the code actually ran inside a virtual machine instead of a conventional container.


### Prerequisites
1. Set up `cloud-init` enabled image which will be used to execute docker images in Qemu/KVM `isolation` mode
```sh
$ sudo cd /var/lib/libvirt/images   
$ sudo wget https://dl.fedoraproject.org/pub/archive/fedora/linux/releases/22/Cloud/x86_64/Images/Fedora-Cloud-Base-22-20150521.x86_64.qcow2
$ sudo mv Fedora-Cloud-Base-22-20150521.x86_64.qcow2 disk.img.orig
```
2. For the system running on `ubuntu` on x86 please install following packages. If you are using distribution other than `ubuntu` please use the corresponding available packages for your distribution. 
```sh
$ sudo apt-get install libvirt-dev qemu-system qemu-system-x86
```
3. Set the `user` for Qemu process run by the system. 
```
$ sudo vim vim /etc/libvirt/qemu.conf
```
Uncomment and set the value of `user`
```
user = "root"
```
4. Since this release compiles the code outside of docker's build container please make sure you are running the [latest version of `golang`](https://golang.org/doc/install#install).  


### Building
```sh
$ mkdir -p $HOME/repos
$ cd $HOME/repos
$ git clone https://github.ibm.com/bpradipt/docker-qemu-isolation.git
$ git checkout -b build_branch --track origin/qemu-isolation
$ hack/make.sh dynbinary
```


### Running the containers in Qemu/KVM isolation


Note that you may need to have `docker-containerd` binary in `PATH` before proceeding. `docker-containerd` gets installed when you install `docker` on your system.


1. Start a `dockerd` from the build process above,
```sh
$ cd $HOME/repos//docker-fork/bundles/1.13.0-dev/dynbinary-daemon
$ sudo dockerd
```
2. In a separate terminal run your first `isolated container`
```sh
$ cd $HOME/repos//docker-fork/dynbinary-client
$ sudo ./docker run --isolation=qemu  busybox /bin/ls
```


### Contributors
```
Abhishek Dasgupta - abdasgupta@in.ibm.com
Sudipto Biswas - sbiswas7@in.ibm.com
Pradipta Kumar -  bpradipt@in.ibm.com
Harshal Patil - harshal.patil@in.ibm.com
```


### TODOs	


Docker logs on isolated containers
Hostname setting using docker containers
Removal of the for loop to determine sync states of containers and the virtual machine with libvirt events.
### License


Apache License 2.0


