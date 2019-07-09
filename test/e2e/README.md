This directory contains end-to-end tests that do not require kubernetes



## Notes on EC2 tests

- set up your ec2 instance
  - download a simple echo app
  - make the app executable
  - run it in the background

```bash
wget https://mitch-solo-public.s3.amazonaws.com/echoapp2
chmod +x echoapp2
sudo ./echoapp2 --port 80 &
```

