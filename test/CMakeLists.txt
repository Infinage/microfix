# Fetch GTest external dependency
include(FetchContent)
FetchContent_Declare(googletest
    GIT_REPOSITORY https://github.com/google/googletest.git
    GIT_TAG 73a63ea05dc8ca29ec1d2c1d66481dd0de1950f1)
set(BUILD_GMOCK OFF CACHE BOOL "" FORCE)
set(INSTALL_GTEST OFF CACHE BOOL "" FORCE)
FetchContent_MakeAvailable(googletest)

include(GoogleTest)
add_executable(fieldTest field_test.cpp)
target_link_libraries(fieldTest PRIVATE field GTest::gtest_main)
gtest_discover_tests(fieldTest)
