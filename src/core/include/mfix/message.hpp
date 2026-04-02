#pragma once

#include "mfix/field.hpp"
#include <expected>
#include <optional>
#include <vector>

namespace mfix {
    struct Message {
        template<typename T, typename R>
        using match_const = std::conditional_t<
            std::is_const_v<std::remove_reference_t<T>>,
            std::add_const_t<R>, R>;

        std::vector<Field> fields;

        [[nodiscard]] static std::expected<Message, std::string> 
        from_string(std::string_view raw, char SEP = '\01');

        Message(std::initializer_list<Field> fields);

        [[nodiscard]] auto begin(this auto &&self) { return self.fields.begin(); }
        [[nodiscard]] auto end(this auto &&self) { return self.fields.end(); }

        [[nodiscard]] bool contains(int tag) const;
        [[nodiscard]] std::optional<std::string> code() const;

        [[nodiscard]] std::size_t size() const;
        [[nodiscard]] std::string to_string(char SEP = '\01') const;

        [[nodiscard]] auto find(this auto &&self, int tag) -> 
            match_const<decltype(self), Field>* 
        {
            auto it = std::ranges::find(self.fields, tag, &Field::tag);
            if (it == self.fields.end()) return nullptr;
            return &*it;
        }
        
        [[nodiscard]] auto findAll(this auto &&self, int tag) -> 
            std::vector<match_const<decltype(self), Field>*>
        {
            using Ret = match_const<decltype(self), Field>*;
            std::vector<Ret> result;
            for (auto &field: self.fields) {
                if (field.tag == tag)
                    result.emplace_back(&field);
            }
            return result;
        }

        [[nodiscard]] auto back(this auto &&self) -> 
            match_const<decltype(self), Field>&
        { return self.fields.back(); }

        void push_back(auto &&...field) {
            fields.emplace_back(std::forward<decltype(field)>(field)...);
        }

        std::size_t erase(int tag);
    };
}
