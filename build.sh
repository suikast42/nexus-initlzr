#!/bin/bash
 if [ x"${PUSH_REGISTRY}" == "x" ]; then
     echo "Build version $APP_VERSION of $APP_NAME and push to dockerhub"
     docker  build --build-arg PULL_REGISTRY=$PULL_REGISTRY --build-arg  CACHE_TS=$(date +%s) -t suikast42/$APP_NAME:$APP_VERSION  .
     echo docker push suikast42/$APP_NAME:$APP_VERSION
     docker push suikast42/$APP_NAME:$APP_VERSION
 else
     echo "Build version $APP_VERSION of $APP_NAME and push to $REGISTRY"
     docker  build --build-arg PULL_REGISTRY=$PULL_REGISTRY  -t  $REGISTRY/suikast42/$APP_NAME:$APP_VERSION .
     echo docker push $PUSH_REGISTRY/suikast42/$APP_NAME:$APP_VERSION
     docker push $PUSH_REGISTRY/suikast42/$APP_NAME:$APP_VERSION
 fi

 