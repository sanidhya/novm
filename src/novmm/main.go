package main

import (
    "flag"
    "log"
    "novmm/loader"
    "novmm/machine"
    "novmm/platform"
)

// Machine specifications.
var vcpu_data = flag.String("vcpus", "[]", "list of vcpu states")
var device_data = flag.String("devices", "[]", "list of device states")

// Linux parameters.
var boot_params = flag.String("setup", "", "linux boot params (vmlinuz)")
var vmlinux = flag.String("vmlinux", "", "linux kernel binary (ELF)")
var initrd = flag.String("initrd", "", "initial ramdisk image")
var cmdline = flag.String("cmdline", "", "linux command line")
var system_map = flag.String("sysmap", "", "kernel symbol map")

// Debug parameters.
var step = flag.Bool("step", false, "step instructions")
var trace = flag.Bool("trace", false, "trace kernel symbols on exit")

func main() {
    // Parse all command line options.
    flag.Parse()

    // Create VM.
    vm, err := platform.NewVm()
    if err != nil {
        log.Fatal(err)
    }
    defer vm.Dispose()

    // Create the machine model.
    model, err := machine.NewModel(vm)
    if err != nil {
        log.Fatal(err)
    }

    // Load all vcpus.
    vcpus, err := vm.LoadVcpus([]byte(*vcpu_data))
    if err != nil {
        log.Fatal(err)
    }
    if len(vcpus) == 0 {
        log.Fatal(NoVcpus)
    }

    // Load all devices.
    err = model.LoadDevices(vm, []byte(*device_data))
    if err != nil {
        log.Fatal(err)
    }

    // Load given kernel and initrd.
    var sysmap loader.SystemMap
    var convention *loader.Convention

    if *vmlinux != "" {
        sysmap, convention, err = loader.LoadLinux(
            vcpus[0],
            model,
            *boot_params,
            *vmlinux,
            *initrd,
            *cmdline,
            *system_map)
        if err != nil {
            log.Fatal(err)
        }
    }

    // Create our tracer with the map and convention.
    tracer := loader.NewTracer(sysmap, convention)
    if *trace {
        tracer.Enable()
    }

    // Start all VCPUs.
    // None of these will actually come online
    // until the primary VCPU below delivers the
    // appropriate IPI to start them up.
    vcpu_err := make(chan error)
    for _, vcpu := range vcpus {
        go func(vcpu *platform.Vcpu) {
            defer vcpu.Dispose()
            err := Loop(vcpu, model, *step, tracer)
            if err != nil {
                vcpu.Dump()
            }
            vcpu_err <- err
        }(vcpu)
    }

    // Wait until we get a signal,
    // or all the VCPUs are dead.
    vcpus_alive := len(vcpus)

    for {
        select {
        case err := <-vcpu_err:
            vcpus_alive -= 1
            if err != nil {
                log.Printf("Vcpu died: %s", err.Error())
            }
        }

        // Everything died?
        if vcpus_alive == 0 {
            log.Fatal(NoVcpus)
        }
    }
}
