#pragma once
#include <string>

namespace mfix {
    struct Field {
        int tag;
        std::string value;

        Field(int tag, std::string value = ""): 
            tag {tag}, value {value} {}
    };
}
