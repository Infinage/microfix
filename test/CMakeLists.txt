# Fetch GTest external dependency
include(FetchContent)
FetchContent_Declare(googletest
    GIT_REPOSITORY https://github.com/google/googletest.git
    GIT_TAG 73a63ea05dc8ca29ec1d2c1d66481dd0de1950f1)
set(BUILD_GMOCK OFF CACHE BOOL "" FORCE)
set(INSTALL_GTEST OFF CACHE BOOL "" FORCE)
FetchContent_MakeAvailable(googletest)

include(GoogleTest)

add_executable(conversion_test conversion_test.cpp)
target_link_libraries(conversion_test PRIVATE converter GTest::gtest_main)
gtest_discover_tests(conversion_test)

add_executable(message_test message_test.cpp)
target_link_libraries(message_test PRIVATE message GTest::gtest_main)
gtest_discover_tests(message_test)
