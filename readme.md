# OffsiteZFSBackup
###### Also does btrfs!

### Backups
  - btrfs can only backup the root partition for now.
  - zfs can backup all zpools
  - one Google Drive folder per subvolume to backup. Otherwise your data might get wiped by a cleanup command.
  - it's all encrypted
  - it can use vault
  - it can restore :)
  - You can do incremental backups from restored volumes if the name stayed the same
  - It only restores snapshots. You need to use them manually (restore subvolume to snapshot, etc.)

### Encryption of backups:

For encryption and authentication 2 different keys are used. These are derived from the passphrase and the IV of the snapshot being up-/downloaded.
The passphrase does not change for a backup, even with multiple snapshots. The IV however is unique to each new snapshot, thus the actual encryption- and authentication-keys will be different for each snapshot, too.
The IV however, is NOT created for each chunk, but once for the whole snapshot. A chunk on its own is worthless and is just to split the upload into multiple files.
All chunks in the correct order are to be considered the ciphertext.
The supported AES modes are all stream-ciphers. AES-CTR is recommended.

The en-/decryption and authentication schemes can be picked by the user. They will just be abbrevieated with ENC, DEC and AUTH respectively.

Thevariables are constucted in the following way:
```
authKey, encryptionKey         = SHA3-512-HKDF(passphrase, perSnapshotIV, "OZB HKDF")

AUTH                           = userDefinedMAC(authKey)
ENC / DEC                      = userDefinedAESMode(encryptionKey, perSnapshotIV)

authentication                 = AUTH(plaintext)
compressed_plaintext           = lz4_compress(plaintext)

ciphertext                     = ENC(compressed_plaintext)

decrypted_compressed_plaintext = DEC(ciphertext)
decrypted_plaintext            = lz4_decompress(decrypted_compressed_plaintext)
```

For identical data at the end of the encryption -> decryption cycle
```
authentication = AUTH(decrypted_plaintext)
```
has to be true

---
### Threat model:

- The attacker has full access to the Google Drive.
- The attacker does not have access to the secrets used for encryption or authentication.
- The attacker does not have access to the system that is being backed up.
- The attacker does not have capabilities to brute-force 2 independent 256-bit keys in the near future.
- The attacker does not know which filesystem-type was used to create the snapshot.
- Data that does not match against the valid `authenticaton` MAC is not considered breached.

####Protections:

##### Data theft:
  ###### mitigation:
  An attacker would require access to the files produced by this application (granted via Google Drive access) and the encryption and authentication secrets (not in the attackers possession).
  All decrypted snapshots would need to be applied against the appropriate filesystem-type.
  ###### brute-force:
  Is possible. An attacker would need to brute-force the encryption- and authentication-keys until the decryption produces valid lz4 data.
  The completely decompressed lz4 data's HMAC would have to match the `authenticaton` MAC for a 1:1 copy of the data.
##### Manipulation of a snapshot/data corruption:
  ###### detection:
  Swapping out a snapshot is impossible, as ZFS or btrfs would deny applying another snapshot ontop of it.
  To Manipulata a chunk, an attacker would need to modify the ciphertext on Google Drive in a way, that it decrypts and decompresses to valid lz4 data and still applies to the filesystem-type of the creation cleanly.
  ZFS and btrfs are picky about snapshot-data. Corrupt data will yield an error during restoring it.
  The snapshot will not be able to be restored if the decrypted and decompressed ciphertext of any chunk is not compliant to either ZFS or btrfs (this information is not known by the attacker).
  The `authentication` MAC is only checked after a full apply of the snapshot as a last line of defense.
  ###### mitigation:
  The victim could try to manually restore the original version of the snapshots metadata and chunks/ciphertext. This however, can be made impossible by the attacker, if it decides to wipe the version history of those files.
##### Data loss due to loss of archives:
  ###### mitigation:
  None, since we rely 100% on storage of a third party and the attacker has full access to delete and modify everything on it.
  ###### detection:
  No backup is found
##### Data loss due to loss of key material:
  ###### mitigation:
  Backup the key material to a secure storage or paper.
  ###### detection:
  Lz4 data may not decompress (because the decryption is using incorrect key material).
  In the unlikely case, that the lz4 data is correct, and applies cleanly to the filesystem the `authentication` MAC will not match `AUTH(decrypted_plaintext)`