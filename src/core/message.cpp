#include "mfix/utils.hpp"
#include "mfix/message.hpp"

#include <algorithm>
#include <sstream>
#include <ranges>

namespace mfix {
    Message::Message(std::initializer_list<Field> fields): 
        fields{fields} {}

    std::size_t Message::size() const { return fields.size(); }

    bool Message::contains(int tag) const {
        return std::ranges::contains(fields, tag, &Field::tag);
    }

    std::optional<std::string> Message::code() const {
        if (auto code = find(35)) return code->value;
        return std::nullopt;
    }

    std::string Message::to_string(char SEP) const {
        std::ostringstream oss;
        for (auto &field: fields) {
            oss << field.tag << "=" << field.value << SEP;
        }
        return oss.str();
    }

    std::size_t Message::erase(int tag) {
        auto remRng = std::ranges::remove(fields, tag, &Field::tag);
        auto erased = static_cast<std::size_t>(
            std::distance(remRng.begin(), remRng.end()));
        fields.erase(remRng.begin(), remRng.end());
        return erased;
    }

    std::expected<Message, std::string> Message::from_string(std::string_view raw, char SEP) {
        auto fieldCounts = static_cast<std::size_t>(std::ranges::count(raw, SEP));
        if (!fieldCounts) return std::unexpected{"No delimiters found"};

        Message result {}; result.fields.reserve(fieldCounts);
        for (auto fRaw: raw | std::views::split(SEP)) {
            std::string_view kv{fRaw.begin(), fRaw.end()};

            if (result.fields.size() == fieldCounts) {
                if (kv.empty()) break;
                return std::unexpected{"Must end with '" + std::string{SEP} + "'"};
            }

            auto eqCount = std::ranges::count(kv, '=');
            if (eqCount != 1)
                return std::unexpected{"Must have exactly one '=': '" + std::string{kv} + "'"};

            auto pos = kv.find('=');
            auto key = _convert<int>(kv.substr(0, pos));
            if (!key.has_value()) 
                return std::unexpected{"Tag not an INT: '" + std::string{kv.substr(0, pos)} + "'"};

            std::string value {kv.substr(pos + 1)};
            result.push_back(*key, value);
        }

        return result;
    }
}
