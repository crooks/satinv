# satinv - Satellite Inventory

This program will generate an [Ansible](https://ansible.com) [Dynamic Inventory](https://docs.ansible.com/ansible/latest/user_guide/intro_dynamic_inventory.html) using [Red Hat Satellite](https://www.redhat.com/en/technologies/management/satellite) as a source.  The Dynamic Inventory will consist of a number of inventory groups:-
* Host Collections - Within Red Hat Satellite, hosts can be assigned to one or more Host Collections.
* Valid Hosts - In this context a host is valid if it has:
    * an OS installed (that Satellite recognises)
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
The location can be overridden with `--config=/path/to/config.yml` or by setting the environment variable `SATINVCFG`.  **Note**: You cannot use the --config option when running satinv from `ansible-playbook` or `ansible-inventory`.  This is a constraint imposed by Ansible.

### Options Overview
#### api
The api section is concerned with accessing the Red Hat Satellite API
* baseurl: URL of the Red Hat Satellite instance.
* certfile: Path to a root certificate file.  Probably only required if the above URL is self-signed.
* user: Username that provides access to the API.  Ideally this should be a low privilege, read-only user.
* password: password for the above user
#### cache
The cache sections deals with how frequently the inventory components should be refreshed
* dir: Directory where the cache files will be stored.
* validity: How long (in seconds) the Satellite API results in the cache are considered valid.  Default: 28800
* inventory_validity: How long (in seconds) the dynamic inventory file is considered valid.  Default: 7200

Note: **inventory_validity** should always be less than **validity**.
#### cidrs
The cidrs section contains a dictionary keyed by inventory_groupname and containing the CIDR of hosts that will occupy that group.
#### valid
The valid section contains settings relating to the special **valid** group.
* days: A host must have reported into Satellite within this number of days to be considered valid.
* exclude_hosts: A list of hostnames that should be excluded
* exclude_regex: A list of Regular Expressions.  A hostname matching any of these will be excluded.

### Example Configuration
```
---
api:
  baseurl: https://www.redhat.mydomain.com
  user: myreadonlyuser
  password: myPassword

cache:
  dir: ~/satinv/cache
  validity: 28800

target_filename: ~/satinv/inventory.json

cidrs:
  dev: 192.168.0.0/24
  test: 192.168.1.0/24
  prod: 192.168.100.0/23

valid:
  days: 5
  exclude_hosts:
    - badhostname
    - dontansibleme
  exclude_regex:
    - ^test
    - test[0-9][0-9]$
```
Test the configuration by running `satinv --debug` (assuming your config path is predefined).

## Usage
To use the dynamic inventory consider the following commands:
* `ansible-inventory -i /usr/local/bin/satinv --list`
* `ansible-playbook -i /usr/local/bin/satinv my_playbook.yml`