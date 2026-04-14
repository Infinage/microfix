#pragma once

#include <string>
#include <expected>
#include <optional>
#include <unordered_map>
#include <vector>

namespace mfix {
    struct Message;

    class Spec {
        public:
            enum class DataType {
                Amount,
                Boolean,
                Char,
                Country,
                Currency,
                Data,
                DayOfMonth,
                Exchange,
                Float,
                Int,
                Language,
                Length,
                LocalMktDate,
                LocalMktTime,
                MonthYear,
                MultipleCharValue,
                MultipleStringValue,
                NumInGroup,
                Percentage,
                Price,
                PriceOffset,
                Quantity,
                SeqNum,
                String,
                TagNum,
                TZTimeOnly,
                TZTimestamp,
                XID,
                XIDRef,
                XMLData,
                UTCDate,
                UTCDateOnly,
                UTCTimeOnly,
                UTCTimestamp,
            };

            struct FieldSpec {
                int tag;
                std::string name;
                DataType dtype;

                // value and description for char types
                std::vector<std::pair<std::string, std::string>> enums;
            };

            // Nested structure of messages - components are flattened out
            struct MessageSpecEntry {
                int tag;
                bool required;
                bool isGroup;
                std::vector<MessageSpecEntry> children {};
            };

            struct ValidationResult {
                std::vector<std::string> observations;

                operator bool() const { return observations.empty(); }

                ValidationResult &observation(const std::string &obs) {
                    observations.push_back(obs);
                    return *this;
                }
            };

            enum class ValidationMode {
                None,   // no validation
                Basic,  // checksum, bodylen, required fields, groups
                Strict, // type check, unknown fields rejected, ordering
            };

            struct SampleOptions {
                bool requiredOnly;
                std::unordered_map<DataType, std::string> defaultValueOverides;
                std::unordered_map<int, int> groupCountOverides;
            };

            // Convenience types
            using MessageSpec = std::vector<MessageSpecEntry>;    
            using Fields = std::unordered_map<int, FieldSpec>; // look up by tag
            using Messages = std::unordered_map<std::string, MessageSpec>; // look up msg code

            // type.major.minor.sp{sp}
            std::string code;

            static std::expected<Spec, std::string> loadSpec(std::string_view fpath);

            std::optional<FieldSpec> field(int tag) const;

            std::optional<Message> sample(const std::string &msgType, SampleOptions options = {true, {}, {}}) const;

            ValidationResult validate(const Message &message, ValidationMode mode) const;

        private:
            Spec() = default;

            std::string type;              
            int major, minor, sp;

            // Headers / trailers - field tag, required
            MessageSpec header, trailer;

            // Lookup by code (not by name): field tag, required
            Messages messages;

            // Lookup by tag - components are flattened
            Fields fields;
    };
}
