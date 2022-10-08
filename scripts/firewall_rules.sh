#!/bin/bash
SERVERS="ubuntu@$MASTER_HOST ubuntu@$SLAVE_HOST"

for i in $SERVERS
do
   ssh -o StrictHostKeyChecking=no $i "\
      ssh-keyscan -t ed25519 $NEW_SERVER_IPV6 > homepage_host_key && \
      sudo ip6tables -A INPUT -s $NEW_SERVER_IPV6 -m conntrack -p tcp --dport 22 --ctstate NEW,ESTABLISHED -j ACCEPT && \
      sudo ip6tables -D INPUT -s $OLD_SERVER_IPV6 -m conntrack -p tcp --dport 22 --ctstate NEW,ESTABLISHED -j ACCEPT && \
      sudo ip6tables-save -f /etc/iptables/rules.v6"
done
