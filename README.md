
# execvm

`execvm` is `execve` for virtual* machines

```
chain
 - chain
  - chain
   -- chain
    -- chain
     --- chain
      --- chain
       ---- chain
        ---- chain
         ----- chain
          ----- chain
           ------ chain
            ------ chain
             ...
```

## Overview

`execvm` is a lot of things - a friend, a pal, a confidant.

I'd like to be able to run single processes inside a VM with the same
interface as running local processes, with file handles mapped
appropriately so that I can include these executions in local shell
pipelines. The intent is for utility/processing type tasks for which
docker isolation isn't quite enough. In particular, I'd like to be
able to run these processes with a root given by some volume and to do
that I need block device emulation.

This package has two components, the go package implementing the
`execvm` cli, plus a python package implementing all these
primitives. Then the "vm" in "execvm" means a really abstract virtual
machine that might be a local executor, docker executor, or
what-you'd-expect-when-i-say-vm vm executor.

Consider the task of compiling the `execvm` alpine package. I'd like
to run that in an isolated build environment, for obvious reasons, and
I'd like the process to be stateless. I especially don't want to have
wrangle volume mounts, because I don't want to be tied to the local
filesystem - this build process should be agnostic to its execution
context, so I can feed it the package description and it can return
the built package from wherever it happens to be running.

Without `execvm` we could do something like this:

```
tar -czf - \
    | docker run \
          /bin/bash -c "tar -xaf - && make && cat pkg" \
    > execvm-0.2.pkg.tar.gz
```

This satisfies our interface requirement, but it requires us to fire
up an entire shell in a container and do string building, which isn't
really very programmable. What if I want to have docker runs of other
commands in pipelines like this? I'll have a different string for
each, and maybe eventually I'll have enough patterns that I'll start
doing icky stuff like `program="$command1 $command2"` to dynamically
generate the command being docker ran.

How will I track that code, those little "tar && make && cat"
scriptlets? In a `Makefile`? Or will I put these notes in a
`README.md` and just accept that next time I want to run this I'll
have to copy/paste, which I do willingly because I only run it rarely?
What if I want to run it more frequently? Having that ability would
make testing easier, since I could spin things up quicker. And what if
I don't want to run those commands in docker?  What if I want to run
them locally or in a vm?

With `execvm` we can instead lean into the kernel's `exec` interface
and represent our entire "program" as a list of strings:

```
tar -czf - \
    | docker run $dockerconfig \
          /bin/execvm chain \
            -- \
            /bin/tar -xaf - \
            -- \
            /bin/execvm chain \
              -- \
              /bin/make \
              -- \
              /bin/cat pkg" \
    > execvm-0.2.pkg.tar.gz
```

Now we can imagine that program generators are not string templaters
but instead simply stateless functions that return arrays of strings!
Thus, we not only avoid issues of string interpolation, but code
generation becomes easier and more trustworthy, so the pipeline itself
becomes less fragile. Moreover, the pipeline description is captured
as data, so changes to the pipeline look just like regular
configuration changes instead of code changes. This is helpful for
scriptlets like these, where we want the benefits of formal source
control without having to spin up an entire build process for a short,
disposable shell script.

Notice that if we have all of these tools (and permissions) available
in our local execution context, we could remove the docker run and
simply:

```
tar -czf - \
    | /bin/execvm chain \
        -- \
        /bin/tar -xaf - \
        -- \
        /bin/execvm chain \
          -- \
          /bin/make \
          -- \
          /bin/cat pkg" \
    > execvm-0.2.pkg.tar.gz
```

This suggests we should have a similar interface for running this
command in a vm:

```
tar -czf - \
    | run-in-vm $vmconfig \
          /bin/execvm chain \
            -- \
            /bin/tar -xaf - \
            -- \
            /bin/execvm chain \
              -- \
              /bin/make \
              -- \
              /bin/cat pkg" \
    > execvm-0.2.pkg.tar.gz
```

That would afford us one api for building commands to run, and another
 to specify an execution context in which to run those commands.

## Tools

### chain

`exec` up to two commands, one after the other

Should you want to sequence multiple commands without any more flow
control than "stop on error", `chain` provides basic process
sequencing without the benefits of a shell. This is accomplished by
defining a simple language on its command line arguments.

To run two commands:

```
$ execvm chain -- /bin/true -- /bin/true
```

The first argument is a delimeter that specifies an interpreter for
parsing the remaining arguments. Starting from the next argument,
everything before the first delimeter is `ForkExec`'d, then everything
after is `Exec`'d. A non-zero exit from the first process will cause
the chain to exit with the same status, e.g. this exits 1 without
running the final `/bin/true`:

```
$ /usr/bin/execvm chain '>>=' \
      /bin/true \
      '>>=' \
      /usr/bin/exec chain '>>=' \
          /bin/false \
          '>>=' \
          /bin/true
```

### init

The `init` command implements what would typically be `/init` in an
`initrd` with a slighty different interface. Rather than code in the
next initialization step (typically an `exec` of `/sbin/init`), we
accept the next command to run in our command line arguments and then
`exec` that.

```
/usr/bin/qemu-system-x86_64 \
    -machine type=pc,accel=kvm \
    -kernel kernel/linux-5.9.12/arch/x86/boot/bzImage \
    -initrd vm-run/initrd2.gz \
    -drive file=vm-run/alpine-iso/alpine-minirootfs-3.12.1-x86_64.iso,media=cdrom \
    -nographic \
    -append """
console=ttyS0
rdinit=/bin/sh
"""
```

By combining these two commands, we can create programmable `init`
scripts without having to "flash" them into a disk image - they can be
passed in dynamically as data in the kernel's command line.

## Building

This creates a new archive combining all of the alpine mini root fs
and `execvm`. The second docker call is to reformat that to a gzipped
cpio archive. The naming is a little off since `execvm` gets installed
in `/execvm/execvm`, for instance. However, we'll be doing this in
python so we can set the archive names appropriately there.

```
tar -czf - \
    execvm/execvm \
    alpine-iso/alpine-minirootfs-3.12.1-x86_64.tar.gz | \
        docker run -i --rm execvm:latest \
            /usr/bin/execvm chain \
              -- \
              /usr/sbin/bsdtar -xzf - \
              -- \
              /usr/sbin/bsdtar -czf - \
              execvm/execvm \
              @alpine-iso/alpine-minirootfs-3.12.1-x86_64.tar.gz | \
            docker run -i --rm archlinux:latest \
                bsdtar --format=newc -czf - @- > initrd.gz
```

## The Python API

We want to pipeline these docker calls in python code! Analogous to a
bind, but closer to the shell's pipe, I use the or operator for those
cases where we want to connect stdout to stdin:

```
computer = docker-runner(image="docker-runner")
block = computer.command("/bin/echo", "hello") \
            | computer.command("/bin/wc", "-c")
```

Notice that we bind the computer to the command at command creation
time. Then the created block has all necessary context to execute
itself later:

```
assert block.run() == "6"
```

Here I'm using a `docker-runner` to run these commands, but we should
also implement an `exec-runner` that does `subprocess.run` calls
(without a shell). And eventually we'll have a `vm-runner`! Note that
in the example above I use different images for each step. What we
want here is to allow different computers to execute different
commands.

We also need a `docker-build` primitive the runs the docker-in-python
build. This, like `tar`, is computer-agnostic, so we could actually
write a `tar | docker-build` pipeline that can be run with a really
basic computer, like one that can't even interpret commands. OR! Maybe
for the sake of debugging, we start with one that has a print-only
executor (instead of invoking docker or subprocessing, etc) for each
encountered command, while still slurping along any python-generated
io? Then pipelines without commands would be silent anyway, so that'd
be a good pick for those build type jobs.

It would be cool to add file io like:

```
block = tar("file1", "file2", compress="gzip") \
            | computer.command("/bin/tar", "-tzf", "-") \
            | computer.command("wc", "-l") > "output"
```

Then the file "output" would contain the string "2".

And note that this is tar-in-python, not a tar being exec'd somewhere.
That lets us read local files, so it needs to be a primitive.

And finally I think we need a `chain()` constructor that takes a
collection of `command` and produces a new command. The computer then
needs to decide which `chain` executable to run at execution
time. Then we'd be able to create command sequences like this:

```
block = computer.command(chain(["/bin/true"],
                               ["/bin/true"])
```

### runners

#### void

doesn't actually run commands, but it does slurp along io. should take
a debug flag to print commands.

#### local

runs locally, i.e. does a `subprocess.run` to do the exec.

#### docker

does a docker run from within python, i.e. for each command we start a
container. container configuration is defined by the runner itself,
which means every container will be the same.

The most robust pipeline I run looks like this first docker call. I
send data via a tarball on stdin, then use chain to unpack that
tarball and run something interesting.

```
tar -czf - \
    execvm/execvm \
    alpine-iso/alpine-minirootfs-3.12.1-x86_64.tar.gz | \
        docker run -i --rm execvm:latest \
            /usr/bin/execvm chain \
              -- \
              /usr/sbin/bsdtar -xzf - \
              -- \
              /usr/sbin/bsdtar -czf - \
              execvm/execvm \
              @alpine-iso/alpine-minirootfs-3.12.1-x86_64.tar.gz | \
            docker run -i --rm archlinux:latest \
                bsdtar --format=newc -czf - @- > initrd.gz
```

#### vm

does a `subprocess.run` to start a `qemu` process. the kernel to use
is an input, so is the initrd. We should provide a usable initrd
definition that is buildable by execvm. For the kernel we'll also
provide an execvm-buildable kernel config for demo'ing a complete
installation. so, on init we call the qemu process builder, which
gives us a runnable process with all the `-initrd` and `-kernel` stuff
set appropriately. would be cool to expose a declarative api for
attaching devices like `devices={"/local/file": "/dev/vda"}`. internal
to that process then is a closure that will generate the `-append`
spec. we'll pass that closure the command to run, which is a list of
strings. we'll jsonify that, b64encode it, then embed it in the param
template. and finally we can do the `subprocess.run`.

### Worked Examples

OK let's remove the Tar and File stuff from the computer's domain -
let's just focus on executing actual Commands, and use a slightly
separate api for managing exeternal IO? In other words, we pipe the
tar into the computer context, then slurp its stdout to map it to a
file? This way the thing-being-computed has an input, a command, and
an output, where the command might be a Pipeline. The type-specific
computation happens on the command, whereas the type-agnostic bits run
the input and output. Maybe also could use ISender and IReceiver
interaces to annotate the tar producer implements ISender, commands
implement both, file writer implements IReceiver?

```
-- stdin --> [   ] -- stdout -->
               |
               |
             stderr
               |
               |
               v
```

```
block = command("/bin/echo", "hello") | command("/bin/wc", "-c")
```

```
Command{
  "args": ["/bin/echo", "hello"],
  "env": {},
  "output": Command{
    "args": ["/bin/wc", "-c"],
    "env": {}
  }
}
```

```
block = tar("file1", "file2", compress="gzip") \
            | command("/bin/tar", "-tzf", "-") \
            | command("wc", "-l") > "output"
```

```
Tar{
  "files": ["file1", "file2"],
  "compression": "gzip",
  "output": Command{
    "args": ["/bin/tar", "-tzf", "-"],
    "env": {},
    "output": Command{
      "args": ["wc", "-l"],
      "env": {},
      "output": File{
        "name": "output"
      }
    }
  }
}
```
