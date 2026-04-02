include(FetchContent)
FetchContent_Declare(pugixml
    GIT_REPOSITORY https://github.com/zeux/pugixml
    GIT_TAG 3070fb6320848a8acdcc205415440a2f2cdb3537
)
include(FetchContent)
FetchContent_Declare(ordered_map
    GIT_REPOSITORY https://github.com/Tessil/ordered-map
    GIT_TAG 3f7ee9abc68ce35047250f3723d7760a5362481e
)
FetchContent_MakeAvailable(pugixml ordered_map)

add_library(field INTERFACE)
target_sources(field INTERFACE FILE_SET HEADERS 
    BASE_DIRS include FILES "include/mfix/field.hpp")

add_library(utils OBJECT)
target_sources(utils PRIVATE utils.cpp 
    PUBLIC FILE_SET HEADERS BASE_DIRS include 
    FILES "include/mfix/utils.hpp")

foreach(lib IN ITEMS converter message)
    add_library(${lib})
    target_link_libraries(${lib} PUBLIC field PRIVATE utils)
    target_sources(${lib} PRIVATE ${lib}.cpp 
        PUBLIC FILE_SET HEADERS BASE_DIRS include 
        FILES "include/mfix/${lib}.hpp")
endforeach()

add_library(spec)
target_sources(spec PRIVATE spec.cpp
    PUBLIC FILE_SET HEADERS BASE_DIRS include
    FILES "include/mfix/spec.hpp")
target_link_libraries(spec message pugixml ordered_map)

# Copying spec files
target_compile_definitions(spec PUBLIC 
    $<BUILD_INTERFACE:MFIX_SPEC_DIR=${PROJECT_SOURCE_DIR}/spec>
    $<INSTALL_INTERFACE:MFIX_SPEC_DIR=${CMAKE_INSTALL_PREFIX}/share/mfix/spec>)
