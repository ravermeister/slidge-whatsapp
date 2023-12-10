#!/usr/bin/env bash
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
    Name-Real: slidge-ci-user
    Name-Comment: slidge build user
    Name-Email: info@rimkus.it
    Expire-Date: 0
    %no-ask-passphrase
    %no-protection
    %pubring pubring.kbx
    %secring trustdb.gpg
    # Do a commit here, so that we can later print "done" :-)
    %commit
    %echo done
EOF

#gpg2 --verbose --batch --gen-key keydetails || exit 1
gpg2 -q --batch --gen-key keydetails || exit 1
if [ "$(gpg2 -k | wc -l)" -eq 0 ]; then
  exit 1
fi

# Set trust to 5 for the key so we can encrypt without prompt.
echo -e "5\ny\n" |  gpg2 --command-fd 0 --expert --edit-key info@rimkus.it trust >/dev/null;

# Test that the key was created and the permission the trust was set.
gpg2 -k



## Test the key can encrypt and decrypt.
#gpg2 -e -a -r info@rimkus.it keydetails
#
## Delete the options and decrypt the original to stdout.
#rm keydetails
#gpg2 -d keydetails.asc
#rm keydetails.asc
