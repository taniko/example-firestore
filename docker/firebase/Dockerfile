FROM node:25.2.1-trixie

RUN apt-get update && \
    apt-get install -y openjdk-25-jre-headless curl --no-install-recommends && \
    rm -rf /var/lib/apt/lists/* && \
    npm install -g firebase-tools

WORKDIR /app

COPY docker/firebase/firebase.json /app/firebase.json
COPY docker/firebase/firestore.indexes.json /app/firestore.indexes.json
COPY docker/firebase/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

EXPOSE 4000 8080 8085

ENTRYPOINT ["/entrypoint.sh"]
