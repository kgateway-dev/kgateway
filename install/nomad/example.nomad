job "example" {

  datacenters = ["dc1"]
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

  group "cache" {
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

    task "redis" {

      driver = "docker"

      config {
        dns_servers = ["172.17.0.1:8600"]
        image = "redis:3.2"
        port_map {
          db = 6379
        }
      }

      resources {
        cpu = 500
        memory = 256
        network {
          mbits = 10
          port "db" {}
        }
      }

      service {
        name = "redis-cache"
        tags = [
          "global",
          "cache"]
        port = "db"
        check {
          name = "alive"
          type = "tcp"
          interval = "10s"
          timeout = "2s"
        }
      }
    }

    task "testrunner" {
      driver = "docker"
      config {
        dns_servers = ["172.17.0.1"]
        image = "soloio/testrunner:testing"
      }

      resources {
        cpu = 500
        memory = 256
        network {
          mbits = 10
        }
      }

      service {
        name = "testrunner"
        tags = [
          "global",
          "cache"]
        check {
          name = "alive"
          type = "script"
          command = "ps"
          interval = "10s"
          timeout = "2s"
        }
      }
    }
  }
}