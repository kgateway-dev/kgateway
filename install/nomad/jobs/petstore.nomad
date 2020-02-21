job "demo" {

  datacenters = ["[[.datacenter]]"]
  type        = "service"

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

  group "petstore" {
    count = 1

    restart {
      attempts = 2
      interval = "30m"
      delay = "15s"
      mode = "fail"
    }

    task "petstore" {

      driver = "docker"
      config {
        image = "soloio/petstore-example:latest"

        [[ if .dockerNetwork ]]
        # Use overlay network
        network_mode = "[[.dockerNetwork]]"
        [[ end ]]

        port_map {
          http = 8080
        }
      }

      resources {
        # cpu required in MHz
        cpu = 100

        # memory required in MB
        memory = 50

        network {
          # bandwidth required in MBits
          mbits = 1
          port "http" {}
        }
      }

      service {
        name = "petstore"
        tags = ["petstore", "demo", "http"]
        port = "http"
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
