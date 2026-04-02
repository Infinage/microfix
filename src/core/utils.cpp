#include "mfix/utils.hpp"
#include <algorithm>

namespace mfix {
    std::uint8_t checksum(std::string_view serialized) {
        auto total = std::ranges::fold_left(serialized, 0u, [](unsigned int acc, char ch) { 
            return (acc + static_cast<unsigned char>(ch));
        });
        return static_cast<std::uint8_t>(total % 256);
    }
}
