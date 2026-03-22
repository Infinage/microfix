#pragma once
#include <string>

namespace mfix {
    struct Field {
        int tag;
        std::string value;
    };
}
