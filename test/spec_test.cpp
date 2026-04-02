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

TEST(SpecTest, FieldLookupPublicApi) {
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
