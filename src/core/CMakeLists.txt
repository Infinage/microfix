include(FetchContent)
FetchContent_Declare(pugixml
    GIT_REPOSITORY https://github.com/zeux/pugixml
    GIT_TAG 3070fb6320848a8acdcc205415440a2f2cdb3537
)
FetchContent_MakeAvailable(pugixml)

add_library(field INTERFACE)
target_sources(field INTERFACE FILE_SET HEADERS 
    BASE_DIRS include FILES "include/mfix/field.hpp")

foreach(lib IN ITEMS converter message)
    add_library(${lib} OBJECT)
    target_sources(${lib} PRIVATE ${lib}.cpp 
        PUBLIC FILE_SET HEADERS BASE_DIRS include 
        FILES "include/mfix/${lib}.hpp")
    target_link_libraries(${lib} PUBLIC field)
endforeach()
