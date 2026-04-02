#pragma once

#include <cstdint>
#include <optional>
#include <string_view>
#include <charconv>

namespace mfix {
    template<typename T> [[nodiscard]] 
    std::optional<T> _convert(std::string_view value) {
        T result {};
        auto *beg = value.data(), *end = beg + value.size();
        auto [ptr, ec] = std::from_chars(beg, end, result);
        if (ec == std::errc{} && ptr == end) return result;
        return std::nullopt;
    }

    std::uint8_t checksum(std::string_view serialized);
}
