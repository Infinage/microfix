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
}
