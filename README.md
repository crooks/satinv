# satinv - Satellite Inventory

This program will generate an [Ansible](https://ansible.com) [Dynamic Inventory](https://docs.ansible.com/ansible/latest/user_guide/intro_dynamic_inventory.html) using [Red Hat Satellite](https://www.redhat.com/en/technologies/management/satellite) as a source.  The Dynamic Inventory will consist of a number of inventory groups:-
* Host Collections - Within Red Hat Satellite, hosts can be assigned to one or more Host Collections.
* Valid Hosts - In this context a host is valid if it has:
    * an IP address (ipv4 or ipv6)
    * a valid Red Hat subscription
    * been seen within an acceptable period of time - 7 days default (config option: sat_valid_days)
* CIDR groups - Groups created based on the subnet the host resides in

## Installation
* Grab a copy of Google's [Go](https://golang.org/) and follow the instructions for your platform to install it.
* Download the **satinv** repository and compile it.
* Copy the resulting binary to somewhere sane (on Linux, /usr/local/bin is probably a good choice).
* Try executing `satinv --help` to check you don't have any runtime errors.

## Configuration
The configuration for **satinv** lives in a single YAML formatted file.  The file can be located anywhere but the default is `/etc/ansible/satinv.yml`.
The location can be overridden with `--config=/path/to/config.yml` or by setting the environment variable `SATINVCFG`.  **Note**: You cannot use the --config option when running satinv from `ansible-playbook` or `ansible-inventory`.  This is a constraint imposed by Ansible.  A basic config file could be:
```
---
api_baseurl: https://www.redhat.mydomain.com
api_user: myreadonlyuser
api_password: myPassword

cache_dir: /var/local/cache/satinv
cache_validity: 28800

target_filename: /tmp/satinv.json

cidrs:
  dev: 192.168.0.0/24
  test: 192.168.1.0/24
  prod: 192.168.100.0/23
```
Test the configuration by running `satinv --debug` (assuming your config path is predefined).

## Usage
To use the dynamic inventory consider the following commands:
* `ansible-inventory -i /usr/local/bin/satinv --list`
* `ansible-playbook -i /usr/local/bin/satinv my_playbook.yml`