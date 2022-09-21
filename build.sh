$Version=1.0.0.Alpha2
docker build .  -t suikast42/nexus-initlzr:$Version
docker push suikast42/nexus-initlzr:$Version