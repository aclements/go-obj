#!/usr/bin/bash

gcc -g -O2 -ffile-prefix-map=$PWD=/ -o inline inline.c inline2.c
