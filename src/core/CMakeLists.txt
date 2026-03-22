include(FetchContent)
FetchContent_Declare(pugixml
    GIT_REPOSITORY https://github.com/zeux/pugixml
    GIT_TAG 3070fb6320848a8acdcc205415440a2f2cdb3537
)
FetchContent_MakeAvailable(pugixml)

add_library(field INTERFACE)
target_sources(field INTERFACE FILE_SET HEADERS 
    BASE_DIRS include FILES "include/mfix/field.hpp")

add_library(converter OBJECT)
target_sources(converter PRIVATE convert.cpp 
    PUBLIC FILE_SET HEADERS BASE_DIRS include 
    FILES "include/mfix/convert.hpp")
target_link_libraries(converter PUBLIC field)
