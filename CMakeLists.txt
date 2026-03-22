cmake_minimum_required(VERSION 4.0)
project(Microfix VERSION 0.0.1 LANGUAGES CXX
    DESCRIPTION "A modern Open Source alternative to MiniFix"
    HOMEPAGE_URL "https://github.com/infinage/microfix")

set(CMAKE_CXX_STANDARD 23)
set(CMAKE_CXX_STANDARD_REQUIRED ON)
set(CMAKE_CXX_EXTENSIONS OFF)
set(CMAKE_EXPORT_COMPILE_COMMANDS ON)

option(MFIX_BUILD_TESTS "Build tests using Google Test" ON)

add_subdirectory(src)

if (MFIX_BUILD_TESTS)
    include(CTest)
    add_subdirectory(test)
endif()
