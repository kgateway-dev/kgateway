job "gloo" {

  datacenters = [
    "dc1"]
  type = "service"

  update {
    max_parallel = 1
    min_healthy_time = "10s"
    healthy_deadline = "3m"
    auto_revert = false
    canary = 0
  }

  migrate {
    max_parallel = 1
    health_check = "checks"
    min_healthy_time = "10s"
    healthy_deadline = "5m"
  }

  group "gloo" {
    count = 1
    restart {
      attempts = 2
      interval = "30m"
      delay = "15s"
      mode = "fail"
    }
    ephemeral_disk {
      size = 300
    }

    # control plane
    task "control-plane" {
      driver = "docker"
      config {
        image = "soloio/control-plane:0.2.0"
        port_map {
          xds = 8081
        }
      }
      resources {
        cpu = 500
        memory = 256
        network {
          mbits = 10
          port "xds" {}
        }
      }
      service {
        name = "control-plane"
        tags = [
          "gloo"]
        port = "xds"
        check {
          name = "alive"
          type = "tcp"
          interval = "10s"
          timeout = "2s"
        }
      }

      # ingress
      task "ingress" {
        driver = "docker"
        config {
          image = "soloio/envoy:v0.1.6-127"
          port_map {
            http = 8080
            https = 8443
            admin = 19000
          }
        }
        resources {
          cpu = 500
          memory = 256
          network {
            mbits = 10
            port "http" {}
            port "https" {}
            port "admin" {}
          }
        }
        service {
          name = "ingress"
          tags = [
            "gloo"]
          port = "admin"
          check {
            name = "alive"
            type = "tcp"
            interval = "10s"
            timeout = "2s"
          }
        }
      }

    }
  }
}