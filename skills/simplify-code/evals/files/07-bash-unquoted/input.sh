#!/bin/bash
# backup.sh SRC DEST : copies *.txt from SRC to DEST, prints count
SRC=$1
DEST=$2
if [ -z $SRC ]; then echo "usage: backup.sh SRC DEST"; exit 2; fi
if [ ! -d $DEST ]; then mkdir -p $DEST; fi
count=0
for f in `ls $SRC | grep '\.txt$'`; do
  cp $SRC/$f $DEST/$f
  count=`expr $count + 1`
done
echo "copied $count files"
