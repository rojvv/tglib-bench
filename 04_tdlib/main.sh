#!/bin/sh
git clone --depth 1 https://github.com/rojvv/tglib-bench-tdlib-build.git
mv tglib-bench-tdlib-build/td .
rm -rf tglib-bench-tdlib-build
TD_PATH=$PWD/td
mkdir build
cd build
cmake -DCMAKE_BUILD_TYPE=Release -DTd_DIR=$TD_PATH/lib/cmake/Td ..
cmake --build . -j 4
cd ..
./build/main
