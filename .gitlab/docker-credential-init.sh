#!/usr/bin/env bash

GPG_USER=
GPG_EMAIL=
GPG_DESC=

gen_gpg_key() {
  rm -rf "${HOME}/.gnupg"
  mkdir -m 0700 "${HOME}/.gnupg"
  touch "${HOME}/.gnupg/gpg.conf"
  chmod 600 "${HOME}/.gnupg/gpg.conf"
  if [ -f "/usr/share/gnupg2/gpg-conf.skel" ]; then
    tail -n +4 /usr/share/gnupg2/gpg-conf.skel > "${HOME}/.gnupg/gpg.conf"
  fi

  cd "${HOME}/.gnupg" || exit 1
  # I removed this line since these are created if a list key is done.
  # touch ${HOME}/.gnupg/{pub,sec}ring.gpg
  gpg2 --list-keys

  cat >keydetails <<EOF
      %echo Generating a basic OpenPGP key
      Key-Type: RSA
      Key-Length: 4096
      Subkey-Type: RSA
      Subkey-Length: 4096
      Name-Real: ${GPG_USER}
      Name-Comment: ${GPG_DESC}
      Name-Email: ${GPG_EMAIL}
      Expire-Date: 0
      %no-ask-passphrase
      %no-protection
      %pubring pubring.kbx
      %secring trustdb.gpg
      # Do a commit here, so that we can later print "done" :-)
      %commit
      %echo done
EOF

  gpg2 --verbose --batch --gen-key keydetails || exit 1
  if [ "$(gpg2 -k | wc -l)" -eq 0 ]; then
    exit 1
  fi

  # Set trust to 5 for the key so we can encrypt without prompt.
  echo -e "5\ny\n" |  gpg2 --batch --command-fd 0 --expert --edit-key "${GPG_EMAIL}" trust;

  # Test that the key was created and the permission the trust was set.
  #gpg2 -k

  ## Test the key can encrypt and decrypt.
  #gpg2 -e -a -r ${GPG_EMAIL} keydetails
  #
  ## Delete the options and decrypt the original to stdout.
  #rm keydetails
  #gpg2 -d keydetails.asc
  #rm keydetails.asc
}

pass_init() {
  gpg_key_fingerprint=$(gpg2 -k "$GPG_EMAIL" | head -n2 | tail -n1 | tr -d " ")  
  pass init "$gpg_key_fingerprint"
}

usage() {
  printf "%s <gpg user> <gpg mail> [<gpg key description>]" "$(basename "$0")"
}

###########################

GPG_USER="$1"
GPG_EMAIL="$2"
GPG_DESC="$3"

if [[ -z "${GPG_USER}" || -z "${GPG_EMAIL}" ]]; then
  usage
  exit 1
fi

if [ -z "${GPG_DESC}" ]; then
  GPG_DESC="generated at $(date '+%Y-%m-%d %H:%I:%S')"
fi

gen_gpg_key
pass_init
