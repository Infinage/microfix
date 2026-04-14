#include <gtest/gtest.h>
#include "mfix/spec.hpp"
#include "mfix/message.hpp"
#include <algorithm>

using namespace mfix;

TEST(SpecTest, ParseTest) {
    auto fixVersions =  {
        "FIX40.xml", "FIX41.xml", "FIX42.xml", "FIX43.xml", "FIX44.xml", 
        "FIX50.xml", "FIX50SP1.xml", "FIX50SP2.xml", "FIXT11.xml"
    };

    for (auto ver: fixVersions) {
        auto spec_res = Spec::loadSpec(ver);
        ASSERT_TRUE(spec_res.has_value()) << spec_res.error();
    }
}

TEST(SpecTest, FieldLookup) {
    auto spec_res = Spec::loadSpec("FIX44.xml");
    ASSERT_TRUE(spec_res.has_value()) << spec_res.error();
    const auto& spec = *spec_res;

    // Test a standard field
    auto msgType = spec.field(35);
    ASSERT_TRUE(msgType.has_value());
    EXPECT_EQ(msgType->name, "MsgType");
    EXPECT_EQ(msgType->dtype, Spec::DataType::String);

    // Test enums within that field
    bool foundEnum = std::ranges::contains(msgType->enums, "D", 
        &std::pair<std::string, std::string>::first);
    EXPECT_TRUE(foundEnum);

    // Test non-existent field
    EXPECT_FALSE(spec.field(99999).has_value());
}

// --- Sample API Tests ---

TEST(SpecTest, SampleRequiredOnly) {
    auto spec_res = Spec::loadSpec("FIX44.xml");
    ASSERT_TRUE(spec_res.has_value()) << spec_res.error();

    // Generate sample for NewOrderList ('E') with only required fields
    Spec::SampleOptions options;
    options.requiredOnly = true;
    
    auto msg = spec_res->sample("E", options);
    ASSERT_TRUE(msg.has_value());

    // List ID mandatory field
    EXPECT_TRUE(msg->contains(66));
    EXPECT_FALSE(msg->contains(390));

    // Part of mandatory component ListOrdGrp
    EXPECT_TRUE(msg->contains(73));
    EXPECT_FALSE(msg->contains(583));
}

TEST(SpecTest, SampleWithRepeatingGroups) {
    auto spec_res = Spec::loadSpec("FIX44.xml");
    ASSERT_TRUE(spec_res.has_value()) << spec_res.error();

    Spec::SampleOptions options;
    options.requiredOnly = false;
    options.groupCountOverides[453] = 2;

    auto msg = spec_res->sample("D", options);
    ASSERT_TRUE(msg.has_value());

    // The counter tag itself must be present and set to "2"
    auto it = msg->find(453);
    ASSERT_TRUE(it != nullptr && it->value == "2");

    // Verify fields inside the PartyID group (like 448, 447, 452) appear
    // Because we have 2 groups, we expect tag 448 to appear twice
    long partyIdCount = std::ranges::count(msg->fields, 448, &Field::tag);
    EXPECT_EQ(partyIdCount, 2);
    long PartyIDSource = std::ranges::count(msg->fields, 447, &Field::tag);
    EXPECT_EQ(PartyIDSource, 2);
    long PartyRole = std::ranges::count(msg->fields, 452, &Field::tag);
    EXPECT_EQ(PartyRole, 2);
}

TEST(SpecTest, SampleDefaultValueOverrides) {
    auto spec_res = Spec::loadSpec("FIX44.xml");
    ASSERT_TRUE(spec_res.has_value()) << spec_res.error();

    // Override what a 'Int' looks like in our sample logon message
    Spec::SampleOptions options;
    options.defaultValueOverides[Spec::DataType::Int] = "42";
    
    auto msg = spec_res->sample("A", options);
    ASSERT_TRUE(msg.has_value());

    // Tag 108 (HeartBtInt) is DataType::Int
    auto it = msg->find(108);
    ASSERT_TRUE(it != nullptr && it->value == "42");
}

TEST(SpecTest, SampleInvalidMsgType) {
    auto spec_res = Spec::loadSpec("FIX44.xml");
    ASSERT_TRUE(spec_res.has_value()) << spec_res.error();
    auto msg = spec_res->sample("xx");
    EXPECT_FALSE(msg.has_value());
}

TEST(SpecTest, Sample_EmptyOptionalGroup) {
    auto spec_res = Spec::loadSpec("FIX44.xml");
    ASSERT_TRUE(spec_res.has_value());
    const auto& spec = *spec_res;

    Spec::SampleOptions options;
    options.requiredOnly = true;

    // NewOrderList
    auto msg = spec.sample("E", options);
    ASSERT_TRUE(msg.has_value());

    // Optional repeating group 453 (NoPartyIDs) should not appear
    EXPECT_FALSE(msg->contains(453));
}

TEST(SpecTest, Sample_NestedGroups) {
    auto spec_res = Spec::loadSpec("FIX44.xml");
    ASSERT_TRUE(spec_res.has_value());
    const auto& spec = *spec_res;

    Spec::SampleOptions options;
    options.requiredOnly = false;
    options.groupCountOverides[453] = 2; // NoPartyIDs
    options.groupCountOverides[802] = 3; // NoPartySubIDs

    auto msg = spec.sample("D", options);
    ASSERT_TRUE(msg.has_value());

    // Top-level group count
    auto topCount = msg->find(453);
    ASSERT_TRUE(topCount != nullptr);
    EXPECT_EQ(topCount->value, "2");

    // Nested multiplicative expansion - PartySubID
    auto subIdCount = std::ranges::count(msg->fields, 523, &Field::tag);
    EXPECT_EQ(subIdCount, 2 * 3); // 2 top-level × 3 nested
}

TEST(SpecTest, Sample_FieldOrdering) {
    auto spec_res = Spec::loadSpec("FIX44.xml");
    ASSERT_TRUE(spec_res.has_value());
    const auto& spec = *spec_res;

    auto msg = spec.sample("D");
    ASSERT_TRUE(msg.has_value());

    // Header fields (8, 9, 35) must come first
    EXPECT_EQ(msg->fields[0].tag, 8);  
    EXPECT_EQ(msg->fields[1].tag, 9);
    EXPECT_EQ(msg->fields[2].tag, 35);

    // Trailer fields (10) must come last
    EXPECT_EQ(msg->back().tag, 10);
}

TEST(SpecTest, Sample_EnumPreferredOverDefault) {
    auto spec_res = Spec::loadSpec("FIX44.xml");
    ASSERT_TRUE(spec_res.has_value());
    const auto& spec = *spec_res;

    auto msg = spec.sample("D"); // NewOrderSingle, tag 54 (Side) has enums
    ASSERT_TRUE(msg.has_value());

    auto it = msg->find(54);
    ASSERT_TRUE(it != nullptr);

    // Must pick first enum value, not a default
    EXPECT_EQ(it->value, "1"); // '1' = Buy in FIX44
}

TEST(SpecTest, Sample_OverrideBeatsEnum) {
    auto spec_res = Spec::loadSpec("FIX44.xml");
    ASSERT_TRUE(spec_res.has_value());
    const auto& spec = *spec_res;

    Spec::SampleOptions options;
    options.defaultValueOverides[Spec::DataType::Char] = "Z";

    auto msg = spec.sample("D", options);
    ASSERT_TRUE(msg.has_value());

    auto it = msg->find(54); // Side
    ASSERT_TRUE(it != nullptr);

    // Override wins over enum
    EXPECT_EQ(it->value, "Z");
}

TEST(SpecTest, Sample_GroupZeroCount) {
    auto spec_res = Spec::loadSpec("FIX44.xml");
    ASSERT_TRUE(spec_res.has_value());
    const auto& spec = *spec_res;

    Spec::SampleOptions options;
    options.groupCountOverides[453] = 0; // NoPartyIDs

    auto msg = spec.sample("D");
    ASSERT_TRUE(msg.has_value());

    auto it = msg->find(453);
    // Either not emitted or value=0
    if (it) {
        EXPECT_EQ(it->value, "0");
        // No children emitted
        EXPECT_EQ(std::ranges::count(msg->fields, 448, &Field::tag), 0);
        EXPECT_EQ(std::ranges::count(msg->fields, 447, &Field::tag), 0);
    } else {
        EXPECT_FALSE(msg->contains(453));
    }
}

TEST(SpecTest, Sample_LargeGroupStress) {
    auto spec_res = Spec::loadSpec("FIX44.xml");
    ASSERT_TRUE(spec_res.has_value());
    const auto& spec = *spec_res;

    Spec::SampleOptions options;
    options.requiredOnly = false;
    options.groupCountOverides[453] = 1000;

    auto msg = spec.sample("D", options);
    ASSERT_TRUE(msg.has_value());

    // Check counter matches
    auto it = msg->find(453);
    ASSERT_TRUE(it != nullptr);
    EXPECT_EQ(it->value, "1000");

    // Spot check first and last group instance
    auto all448 = msg->findAll(448);
    EXPECT_EQ(all448.size(), 1000);
}
