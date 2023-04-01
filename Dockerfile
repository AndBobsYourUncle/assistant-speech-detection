FROM ubuntu:22.04

ENV DEBIAN_FRONTEND="noninteractive"

RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        git \
        wget \
        build-essential gcc make cmake \
        ca-certificates \
        libsdl2-dev \
        libopenblas-dev \
        libportaudio2 \
        portaudio19-dev \
        golang-go && \
    apt-get clean && \
    update-ca-certificates && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /tmp

RUN git clone https://github.com/ggerganov/whisper.cpp && \
    cd whisper.cpp && \
    cmake . && \
    make && \
    make install && \
    cd ../ && \
    rm -r whisper.cpp

RUN echo "/usr/local/lib" > /etc/ld.so.conf.d/local.conf
RUN ldconfig

COPY . /root/assistant_speech_detection

WORKDIR /root/assistant_speech_detection

RUN bash ./models/download-model.sh base.en && bash ./models/download-model.sh tiny.en

#RUN go build

#RUN chmod +x entrypoint.sh

#CMD [ "/bin/bash", "-c", "/root/speech_commands/entrypoint.sh" ]