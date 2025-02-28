# kubeauto

The project reduces the difficulty of running and debugging Kubernetes locally (e.g. using Kind or K3s);

- It automatically displays the status of the resource, and the status is updated in real time.
- It automatically tails logs from pods as they're created.
- It automatically forwards ports to the local machine, as they're created.

I wrote it because of buggy scripts that use `kubetl logs -l` and `kube port-forward`.

## Installation

Find the latest release on the releases page:

```bash
sudo curl --fail --silent --location --output /usr/local/bin/kubeauto \
  https://github.com/kitproj/kubeauto/releases/download/v0.0.12/kubeauto_v0.0.12_linux_amd64 && \
  sudo chmod +x /usr/local/bin/kubeauto
```

For Go users

```bash
go install github.com/kitproj/kubeauto@v0.0.12
```

## Usage

```bash
Usage of kubeauto:
  -g string
        the group to watch, defaults to core resources
  -h    print help
  -l string
        comma separated list of labels to filter resources, e.g. app=nginx, defaults to all resources
  -n string
        namespace to filter resources, defaults to the current namespace 
  -p int
        the offset to add to the host port, e.g. if the container listens on 8080 and the host port is 30000, the offset is 38080, defaults to 30000 (default 30000)

```

```bash
acollins8@macos-QH70WPJVXY kubeauto % k create ns my-ns
namespace/my-ns created

acollins8@macos-QH70WPJVXY kubeauto % k apply -n my-ns -f testdata
...
acollins8@macos-QH70WPJVXY kubeauto % go run .  -n my-ns -l app=nginx -p 40000 
[pods/nginx-77d8468669-4d8qr] (Running) : 
[pods/nginx-77d8468669-zstvn] (Running) : 
...
[pods/nginx-77d8468669-zstvn] nginx port-forwarding 80 -> 40080
[pods/nginx-77d8468669-zstvn] nginx:  Forwarding from 127.0.0.1:40080 -> 80
[pods/nginx-77d8468669-zstvn] nginx:  Forwarding from [::1]:40080 -> 80
... 
```

```bash
```
