# Mist
A simple http request forwarding server

## Overview
Mist is a service that accepts incoming http requests and forwards them to another
service based on host matching.

## Installation
Mist is go getable

    go get github.com/sekhat/mist

## Configuring Mist

By default mist loads host pattern to forward address maps from a file called
mist.conf from the current directory.

mist.conf file is a simple json dictionary, where keys are the host pattern and the
values are the address to forward to. Worth noting, the hostname matching algorithm
will check for matches in the order they are defined in the config file.

### Example

    {
        "*.example.com":"127.0.0.1:1234"
    }

## Host Patterns

### Exact host match

    example.com

This will only match a hostname of `example.com`

### Subdomain wildcard host match

    *.example.com

This will match `example.com` or any subdomain of it

## Command Line Flag

    --mappingConfig=<config file>
        default: mist.conf
        
        Sets the file in which to load host to address mappings from 

    --listenAddr=<listen address>
        default: :80
    
        Sets the listen address and port that mist should listen for
        incoming connections on