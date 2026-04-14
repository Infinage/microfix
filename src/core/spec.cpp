#include "mfix/spec.hpp"
#include "mfix/message.hpp"

#include <filesystem>
#include <format>
#include <stack>

#include <tsl/ordered_map.h>
#include <pugixml.hpp>

#define XSTR(x) STR(x)
#define STR(x) #x
#ifdef MFIX_SPEC_DIR
#pragma message("MFIX_SPEC_DIR set to \"" XSTR(MFIX_SPEC_DIR) "\"")
auto SpecDir = std::filesystem::path{XSTR(MFIX_SPEC_DIR)};
#else
auto SpecDir = std::filesystem::current_path() / "spec";
#endif

namespace {
    using namespace mfix;

    std::string debug(const pugi::xml_node &node) {
        // $ awk -b '{ nb += length + 1 } nb >= <offset> { print; exit }' spec/FIX44.xml
        return node.path() + "::" + std::to_string(node.offset_debug());
    }

    struct ComponentSpecEntry {
        int tag;
        bool required;
        bool isGroup;
        std::vector<ComponentSpecEntry> children {};
        bool isComponent = false;
        std::string componentName;
    };

    using ComponentSpec = std::vector<ComponentSpecEntry>;

    std::optional<Spec::DataType> datatype(std::string_view type_) {
        auto type = type_
            | std::views::transform([](char ch){ return std::tolower(ch); }) 
            | std::ranges::to<std::string>();

        using enum Spec::DataType;
        if (type == "amt") return Amount;
        if (type == "boolean") return Boolean;
        if (type == "char") return Char;
        if (type == "country") return Country;
        if (type == "currency") return Currency;
        if (type == "data") return Data;
        if (type == "dayofmonth") return DayOfMonth;
        if (type == "exchange") return Exchange;
        if (type == "float") return Float;
        if (type == "int") return Int;
        if (type == "language") return Language;
        if (type == "length") return Length;
        if (type == "date" || type == "localmktdate") return LocalMktDate;
        if (type == "time" || type == "localmkttime") return LocalMktTime;
        if (type == "monthyear") return MonthYear;
        if (type == "multiplecharvalue") return MultipleCharValue;
        if (type == "multiplestringvalue" || type == "multiplevaluestring") 
            return MultipleStringValue;
        if (type == "numingroup") return NumInGroup;
        if (type == "percentage") return Percentage;
        if (type == "price") return Price;
        if (type == "priceoffset") return PriceOffset;
        if (type == "qty") return Quantity;
        if (type == "seqnum") return SeqNum;
        if (type == "string") return String;
        if (type == "tagnum") return TagNum;
        if (type == "tztimeonly") return TZTimeOnly;
        if (type == "tztimestamp") return TZTimestamp;
        if (type == "xid") return XID;
        if (type == "xidref") return XIDRef;
        if (type == "xmldata") return XMLData;
        if (type == "utcdate") return UTCDate;
        if (type == "utcdateonly") return UTCDateOnly;
        if (type == "utctimeonly") return UTCTimeOnly;
        if (type == "utctimestamp") return UTCTimestamp;
        return std::nullopt;
    }   

    std::expected<std::pair<Spec::Fields, std::unordered_map<std::string, int>>, std::string> 
    extractFields(const pugi::xml_node &fieldsXML) {
        // Container to hold results
        Spec::Fields fields;
        std::unordered_map<std::string, int> fieldLookup;

        for (const auto &fnode: fieldsXML) {
            auto tag = fnode.attribute("number").as_int(-1);
            if (tag == -1) 
                return std::unexpected{"'number' attr missing in field at " + debug(fnode)};

            auto name = std::string{fnode.attribute("name").as_string()};
            if (name.empty()) 
                return std::unexpected{"'name' attr missing in field at " + debug(fnode)};

            auto type = datatype(fnode.attribute("type").as_string());
            if (!type.has_value())
                return std::unexpected{"'type' attr missing or illdefined in field at " + debug(fnode)};

            std::vector<std::pair<std::string, std::string>> enums;
            for (const auto &pvalue: fnode.children("value")) {
                auto enumv = std::string{pvalue.attribute("enum").as_string()};
                auto enumd = std::string{pvalue.attribute("description").as_string()};
                if (enumv.empty() || enumd.empty())
                    return std::unexpected{"field.value is missing 'enum' or 'description"};
                enums.emplace_back(enumv, enumd);
            }

            fieldLookup.emplace(name, tag);
            fields.emplace(tag, Spec::FieldSpec{tag, name, *type, enums});
        }

        return std::make_pair(fields, fieldLookup);
    }

    std::expected<std::unordered_map<std::string, ComponentSpec>, std::string>
    extractComponents(const pugi::xml_node &componentsXML, 
        const std::unordered_map<std::string, int> &fieldsLookup) 
    {
        auto extractChildren = [&fieldsLookup](auto &&self, const pugi::xml_node &parent) 
            -> std::expected<ComponentSpec, std::string> 
        {
            ComponentSpec specEntries {};
            for (const auto &child: parent.children()) {
                auto type = std::string{child.name()};
                auto name = std::string{child.attribute("name").as_string()};
                auto required = std::string{child.attribute("required").as_string()};
                if (type.empty() || name.empty() || required.empty())
                    return std::unexpected{"Component entry missing tag name or attribute name/required at " + debug(child)};

                ComponentSpecEntry entry {};
                entry.required = required == "Y"; 
                entry.isGroup = false;

                if (type == "field" || type == "group") {
                    auto it = fieldsLookup.find(name);
                    if (it == fieldsLookup.end())
                        return std::unexpected{"Field name not found: " + name};
                    entry.tag = it->second;

                    if (type == "group") {
                        entry.isGroup = true;
                        std::expected<ComponentSpec, std::string> groupEntries = self(self, child);
                        if (!groupEntries.has_value())
                            return std::unexpected{groupEntries.error()};
                        entry.children = std::move(*groupEntries);
                    }
                }

                else if (type == "component") {
                    entry.isComponent = true;
                    entry.componentName = name;
                }

                else {
                    return std::unexpected{"Unknown component entry type: " + type};
                }

                specEntries.push_back(std::move(entry));
            }

            return specEntries;
        };

        // Lookup by component name - don't flatten yet
        std::unordered_map<std::string, ComponentSpec> components;
        for (const auto &cnode: componentsXML) {
            auto name = std::string{cnode.attribute("name").as_string()};     
            if (name.empty()) {
                return std::unexpected{"Component missing attribute name at " + debug(cnode)};
            }

            auto childEntries = extractChildren(extractChildren, cnode);
            if (!childEntries.has_value())
                return std::unexpected{childEntries.error()};
            components[name] = *childEntries;
        }

        return components;
    }

    std::expected<std::unordered_map<std::string, Spec::MessageSpec>, std::string>
    flattenComponents(const std::unordered_map<std::string, ComponentSpec> &components) {
        // Caching nested components and the result container
        std::unordered_map<std::string, Spec::MessageSpec> flattenedResult;

        auto flattenComponent = [&flattenedResult, &components]
        (auto &&self, const ComponentSpec &component) 
            -> std::expected<Spec::MessageSpec, std::string>
        {
            Spec::MessageSpec entries;
            for (const auto &cEntry: component) {
                if (cEntry.isComponent) {
                    auto cname = cEntry.componentName;

                    auto componentIt = components.find(cname);
                    if (componentIt == components.end())
                        return std::unexpected{"Component name not found: " + cname};

                    if (!flattenedResult.contains(cname)) {
                        flattenedResult.insert({cname, {}}); // Handle cyclical deps
                        auto children = self(self, componentIt->second);
                        if (!children.has_value()) return std::unexpected{children.error()};
                        flattenedResult.at(cname) = std::move(*children);
                    }

                    const auto &flattenedCResult = flattenedResult.at(cname);
                    if (flattenedCResult.empty())
                        return std::unexpected{"Empty component, probably cyclical: " + cname};
                    for (const auto &scEntry: flattenedCResult)
                        entries.push_back(scEntry);
                }

                else if (cEntry.isGroup) {
                    auto children = self(self, cEntry.children);
                    if (!children.has_value()) return std::unexpected{children.error()};
                    entries.push_back({.tag = cEntry.tag, .required = cEntry.required, 
                        .isGroup = true, .children = *children});
                }

                else {
                    entries.push_back({.tag = cEntry.tag, .required = cEntry.required, 
                            .isGroup = false, .children = {}});
                }
            }

            return entries;
        };

        for (const auto &[name, spec]: components) {
            if (!flattenedResult.contains(name)) {
                auto flattenedCResult = flattenComponent(flattenComponent, spec);
                if (!flattenedCResult.has_value()) 
                    return std::unexpected{flattenedCResult.error()};
                flattenedResult.insert({name, std::move(*flattenedCResult)});
            }
        }

        return flattenedResult;
    }

    std::expected<Spec::MessageSpec, std::string>
    extractMessageEntries(const pugi::xml_node &xml, 
        const std::unordered_map<std::string, Spec::MessageSpec> &components,
        const std::unordered_map<std::string, int> &fieldsLookup) 
    {
        Spec::MessageSpec spec {};
        for (const auto &node: xml) {
            auto type = std::string{node.name()};
            auto name = std::string{node.attribute("name").as_string()};
            auto required = std::string{node.attribute("required").as_string()};
            if (type.empty() || name.empty() || required.empty()) {
                return std::unexpected{"Entry missing tag name or attribute name/required at " + debug(node)};
            }

            Spec::MessageSpecEntry entry;
            entry.required = required == "Y"; 
            entry.isGroup = false;

            if (type == "field" || type == "group") {
                auto it = fieldsLookup.find(name);
                if (it == fieldsLookup.end())
                    return std::unexpected{"Field name not found: " + name};
                entry.tag = it->second;

                if (type == "group") {
                    entry.isGroup = true;
                    std::expected<Spec::MessageSpec, std::string> groupEntries = 
                        extractMessageEntries(node, components, fieldsLookup);
                    if (!groupEntries.has_value())
                        return std::unexpected{groupEntries.error()};
                    entry.children = std::move(*groupEntries);
                }

                spec.emplace_back(std::move(entry));  
            }

            else if (type == "component") {
                auto it = components.find(name);
                if (it == components.end())
                    return std::unexpected{"Component name not found: " + name};

                // Copy by value
                for (auto cEntry: it->second) {
                    cEntry.required &= entry.required;
                    spec.emplace_back(cEntry);
                }
            }

            else {
                return std::unexpected{"Unknown entry type: " + type};
            }

        }

        return spec;
    }

    static const std::unordered_map<mfix::Spec::DataType, std::string> 
    defaultValues {
        { Spec::DataType::Amount,              "7.0466" },
        { Spec::DataType::Boolean,             "N" },
        { Spec::DataType::Char,                "C" },
        { Spec::DataType::Country,             "IN" },
        { Spec::DataType::Currency,            "INR" },
        { Spec::DataType::Data,                "\0\0" },
        { Spec::DataType::DayOfMonth,          "4" },
        { Spec::DataType::Exchange,            "XXXX" },
        { Spec::DataType::Float,               "7.04" },
        { Spec::DataType::Int,                 "7" },
        { Spec::DataType::Language,            "en" },
        { Spec::DataType::Length,              "0" },
        { Spec::DataType::LocalMktDate,        "20260407" },
        { Spec::DataType::LocalMktTime,        "12:00:00" },
        { Spec::DataType::MonthYear,           "202604" },
        { Spec::DataType::MultipleCharValue,   "A B" },
        { Spec::DataType::MultipleStringValue, "AB CD" },
        { Spec::DataType::NumInGroup,          "1"},
        { Spec::DataType::Percentage,          "0.74" },
        { Spec::DataType::Price,               "70.466" },
        { Spec::DataType::PriceOffset,         "0" },
        { Spec::DataType::Quantity,            "0" },
        { Spec::DataType::SeqNum,              "0" },
        { Spec::DataType::String,              "" },
        { Spec::DataType::TagNum,              "0" },
        { Spec::DataType::TZTimeOnly,          "12:00:00Z" },
        { Spec::DataType::TZTimestamp,         "20260404-12:00:00Z" },
        { Spec::DataType::XID,                 "" },
        { Spec::DataType::XIDRef,              "" },
        { Spec::DataType::XMLData,             "" },
        { Spec::DataType::UTCDate,             "20260407" },
        { Spec::DataType::UTCDateOnly,         "20260407" },
        { Spec::DataType::UTCTimeOnly,         "07:00:03" },
        { Spec::DataType::UTCTimestamp,        "20260407-07:00:03" },
    };
} // annoymous namespace

namespace mfix {
    // Parse the components first and store to a temporary unordered_map
    // Flatten out the components while populating the messages container
    std::expected<Spec, std::string>
    Spec::loadSpec(std::string_view path) {
        // If user provides filename like "FIX44.xml", match against existing specs
        auto fpath = std::filesystem::path{path};
        if (!fpath.has_parent_path() && !std::filesystem::exists(fpath)) {
            fpath = SpecDir / path;
            if (!std::filesystem::exists(fpath))
                return std::unexpected{"File not found in CWD or spec directory: " 
                    + std::filesystem::current_path().string()};
        }

        pugi::xml_document doc;
        if (!doc.load_file(fpath.string().data()))
            return std::unexpected{"Failed to load spec file: " + std::string{fpath}};

        // Private default constructor
        Spec spec;

        // Spec metadata
        spec.type = doc.child("fix").attribute("type").as_string();
        spec.major = doc.child("fix").attribute("major").as_int();
        spec.minor = doc.child("fix").attribute("minor").as_int();
        spec.sp = doc.child("fix").attribute("servicepack").as_int();
        spec.code = std::format("{}.{}.{}", spec.type, spec.major, spec.minor);

        // Parse the fields
        auto fields = extractFields(doc.child("fix").child("fields"));
        if (!fields) return std::unexpected{fields.error()};
        auto fieldsLookup = std::move(fields->second);
        spec.fields = std::move(fields->first);

        // Parse the components first to resolve dependencies
        auto components = extractComponents(doc.child("fix").child("components"), fieldsLookup);
        if (!components) return std::unexpected{components.error()};

        // Flatten components
        auto flattenedComponents = flattenComponents(*components);
        if (!flattenedComponents) return std::unexpected{flattenedComponents.error()};

        // Parse the enums in msgtypes for quicker lookups
        auto msgTypesIT = spec.fields.find(35);
        if (msgTypesIT == spec.fields.end()) 
            return std::unexpected{"Msg types not found"};
        std::unordered_map<std::string, std::string> msgTypes {
            msgTypesIT->second.enums.begin(), // enum value / code
            msgTypesIT->second.enums.end()    // description
        };

        // Parse headers
        auto header = extractMessageEntries(doc.child("fix").child("header"), *flattenedComponents, fieldsLookup);
        if (!header.has_value()) return std::unexpected{header.error()};
        spec.header = *header;

        // Parse trailer
        auto trailer = extractMessageEntries(doc.child("fix").child("trailer"), *flattenedComponents, fieldsLookup);
        if (!trailer.has_value()) return std::unexpected{trailer.error()};
        spec.trailer = *trailer;

        // Parse the messages
        for (const auto &messageXML: doc.child("fix").child("messages").children()) {
            auto name = messageXML.attribute("name").as_string();
            auto msgCode = std::string{messageXML.attribute("msgtype").as_string()};
            auto it = msgTypes.find(msgCode);
            if (it == msgTypes.end())
                return std::unexpected{"Unknown msgtype: " + msgCode + ", " + name};
            auto entries = extractMessageEntries(messageXML, *flattenedComponents, fieldsLookup);
            if (!entries) return std::unexpected{entries.error()};
            spec.messages[msgCode] = *entries;
        }

        return spec;
    }

    std::optional<Spec::FieldSpec> Spec::field(int tag) const {
        auto it = fields.find(tag);
        if (it == fields.end()) return std::nullopt;
        return it->second;
    }

    std::optional<Message> Spec::sample(const std::string &msgType, SampleOptions options) const {
        auto specIt = messages.find(msgType);
        if (specIt == messages.end()) return std::nullopt;

        // Current message spec, position, group counts to repeat
        struct Frame { const MessageSpec& spec; std::size_t pos; std::size_t repeat; };

        // Header first, message and finally trailer (in reverse for stack)
        std::stack<Frame> stk {{{trailer, 0, 1}, {specIt->second, 0, 1}, {header, 0, 1}}};

        Message sample {};
        while (!stk.empty()) {
            auto frame = std::move(stk.top()); stk.pop();

            // Base case
            if (frame.repeat == 0) continue;

            // Repeat while we still have nogroups
            if (frame.pos == frame.spec.size()) {
                if (--frame.repeat) { frame.pos = 0; stk.push(frame); }
                continue;
            }

            // Continue where we had left off - insert only if we would need to
            const auto &entry = frame.spec[frame.pos++];
            if (frame.pos != frame.spec.size() || frame.repeat > 1)
                stk.push(frame);

            // Skip if entry is not required
            if (!entry.required && options.requiredOnly) continue;

            // If group, insert into stack and recurse
            if (entry.isGroup) {
                auto it = options.groupCountOverides.find(entry.tag);
                auto count = it == options.groupCountOverides.end()? 1ul: 
                    static_cast<std::size_t>(it->second);
                sample.push_back(Field{entry.tag, std::to_string(count)});
                stk.push({entry.children, 0, count});
            }

            // Not a group, translate into dummy values
            // If no override: pick dtype specific dummy val or first enum from spec
            else {
                auto fieldSpec = fields.at(entry.tag);
                auto defaultValue = !fieldSpec.enums.empty()? 
                    fieldSpec.enums[0].first: defaultValues.at(fieldSpec.dtype);
                auto it = options.defaultValueOverides.find(fieldSpec.dtype);
                std::string value = it == options.defaultValueOverides.end()? defaultValue: it->second;
                sample.push_back(Field{entry.tag, value}); 
            }
        }

        return sample;
    }
}
