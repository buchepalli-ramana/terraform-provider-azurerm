package appservice

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Azure/azure-sdk-for-go/services/web/mgmt/2021-02-01/web"
	"github.com/google/uuid"
	"github.com/hashicorp/go-azure-helpers/resourcemanager/commonids"
	"github.com/hashicorp/go-azure-helpers/resourcemanager/commonschema"
	"github.com/hashicorp/terraform-provider-azurerm/internal/sdk"
	"github.com/hashicorp/terraform-provider-azurerm/internal/services/appservice/helpers"
	"github.com/hashicorp/terraform-provider-azurerm/internal/services/appservice/parse"
	"github.com/hashicorp/terraform-provider-azurerm/internal/services/appservice/validate"
	kvValidate "github.com/hashicorp/terraform-provider-azurerm/internal/services/keyvault/validate"
	storageValidate "github.com/hashicorp/terraform-provider-azurerm/internal/services/storage/validate"
	"github.com/hashicorp/terraform-provider-azurerm/internal/tags"
	"github.com/hashicorp/terraform-provider-azurerm/internal/tf/pluginsdk"
	"github.com/hashicorp/terraform-provider-azurerm/internal/tf/validation"
	"github.com/hashicorp/terraform-provider-azurerm/utils"
)

type LinuxFunctionAppSlotResource struct{}

type LinuxFunctionAppSlotModel struct {
	Name                          string                                   `tfschema:"name"`
	FunctionAppID                 string                                   `tfschema:"function_app_id"`
	StorageAccountName            string                                   `tfschema:"storage_account_name"`
	StorageAccountKey             string                                   `tfschema:"storage_account_access_key"`
	StorageUsesMSI                bool                                     `tfschema:"storage_uses_managed_identity"` // Storage uses MSI not account key
	StorageKeyVaultSecretID       string                                   `tfschema:"storage_key_vault_secret_id"`
	AppSettings                   map[string]string                        `tfschema:"app_settings"`
	AuthSettings                  []helpers.AuthSettings                   `tfschema:"auth_settings"`
	Backup                        []helpers.Backup                         `tfschema:"backup"` // Not supported on Dynamic or Basic plans
	BuiltinLogging                bool                                     `tfschema:"builtin_logging_enabled"`
	ClientCertEnabled             bool                                     `tfschema:"client_certificate_enabled"`
	ClientCertMode                string                                   `tfschema:"client_certificate_mode"`
	ConnectionStrings             []helpers.ConnectionString               `tfschema:"connection_string"`
	DailyMemoryTimeQuota          int                                      `tfschema:"daily_memory_time_quota"` // TODO - Value ignored in for linux apps, even in Consumption plans?
	Enabled                       bool                                     `tfschema:"enabled"`
	FunctionExtensionsVersion     string                                   `tfschema:"functions_extension_version"`
	ForceDisableContentShare      bool                                     `tfschema:"content_share_force_disabled"`
	HttpsOnly                     bool                                     `tfschema:"https_only"`
	KeyVaultReferenceIdentityID   string                                   `tfschema:"key_vault_reference_identity_id"`
	SiteConfig                    []helpers.SiteConfigLinuxFunctionAppSlot `tfschema:"site_config"`
	Tags                          map[string]string                        `tfschema:"tags"`
	CustomDomainVerificationId    string                                   `tfschema:"custom_domain_verification_id"`
	DefaultHostname               string                                   `tfschema:"default_hostname"`
	Kind                          string                                   `tfschema:"kind"`
	OutboundIPAddresses           string                                   `tfschema:"outbound_ip_addresses"`
	OutboundIPAddressList         []string                                 `tfschema:"outbound_ip_address_list"`
	PossibleOutboundIPAddresses   string                                   `tfschema:"possible_outbound_ip_addresses"`
	PossibleOutboundIPAddressList []string                                 `tfschema:"possible_outbound_ip_address_list"`
	SiteCredentials               []helpers.SiteCredential                 `tfschema:"site_credential"`
}

var _ sdk.ResourceWithUpdate = LinuxFunctionAppSlotResource{}

func (r LinuxFunctionAppSlotResource) ModelObject() interface{} {
	return &LinuxFunctionAppSlotModel{}
}

func (r LinuxFunctionAppSlotResource) ResourceType() string {
	return "azurerm_linux_function_app_slot"
}

func (r LinuxFunctionAppSlotResource) IDValidationFunc() pluginsdk.SchemaValidateFunc {
	return validate.FunctionAppSlotID
}

func (r LinuxFunctionAppSlotResource) Arguments() map[string]*pluginsdk.Schema {
	return map[string]*pluginsdk.Schema{
		"name": {
			Type:         pluginsdk.TypeString,
			Required:     true,
			ForceNew:     true,
			ValidateFunc: validate.WebAppName,
			Description:  "Specifies the name of the Function App Slot.",
		},

		"function_app_id": {
			Type:         pluginsdk.TypeString,
			Required:     true,
			ForceNew:     true,
			ValidateFunc: validate.FunctionAppID,
			Description:  "The ID of the Linux Function App this Slot is a member of.",
		},

		"storage_account_name": {
			Type:         pluginsdk.TypeString,
			Optional:     true,
			ValidateFunc: storageValidate.StorageAccountName,
			Description:  "The backend storage account name which will be used by this Function App Slot.",
			ExactlyOneOf: []string{
				"storage_account_name",
				"storage_key_vault_secret_id",
			},
		},

		"storage_account_access_key": {
			Type:         pluginsdk.TypeString,
			Optional:     true,
			Sensitive:    true,
			ValidateFunc: validation.NoZeroValues,
			ConflictsWith: []string{
				"storage_uses_managed_identity",
				"storage_key_vault_secret_id",
			},
			Description: "The access key which will be used to access the storage account for the Function App Slot.",
		},

		"storage_uses_managed_identity": {
			Type:     pluginsdk.TypeBool,
			Optional: true,
			Default:  false,
			ConflictsWith: []string{
				"storage_account_access_key",
				"storage_key_vault_secret_id",
			},
			Description: "Should the Function App Slot use its Managed Identity to access storage?",
		},

		"storage_key_vault_secret_id": {
			Type:         pluginsdk.TypeString,
			Optional:     true,
			ValidateFunc: kvValidate.NestedItemIdWithOptionalVersion,
			ExactlyOneOf: []string{
				"storage_account_name",
				"storage_key_vault_secret_id",
			},
			Description: "The Key Vault Secret ID, including version, that contains the Connection String to connect to the storage account for this Function App.",
		},

		"app_settings": {
			Type:     pluginsdk.TypeMap,
			Optional: true,
			Elem: &pluginsdk.Schema{
				Type: pluginsdk.TypeString,
			},
			Description: "A map of key-value pairs for [App Settings](https://docs.microsoft.com/en-us/azure/azure-functions/functions-app-settings) and custom values.",
		},

		"auth_settings": helpers.AuthSettingsSchema(),

		"backup": helpers.BackupSchema(),

		"builtin_logging_enabled": {
			Type:        pluginsdk.TypeBool,
			Optional:    true,
			Default:     true,
			Description: "Should built in logging be enabled. Configures `AzureWebJobsDashboard` app setting based on the configured storage setting.",
		},

		"client_certificate_enabled": {
			Type:        pluginsdk.TypeBool,
			Optional:    true,
			Default:     false,
			Description: "Should the Function App Slot use Client Certificates.",
		},

		"client_certificate_mode": {
			Type:     pluginsdk.TypeString,
			Optional: true,
			Default:  web.ClientCertModeOptional,
			ValidateFunc: validation.StringInSlice([]string{
				string(web.ClientCertModeOptional),
				string(web.ClientCertModeRequired),
				string(web.ClientCertModeOptionalInteractiveUser),
			}, false),
			Description: "The mode of the Function App Slot's client certificates requirement for incoming requests. Possible values are `Required`, `Optional`, and `OptionalInteractiveUser`.",
		},

		"connection_string": helpers.ConnectionStringSchema(),

		"daily_memory_time_quota": {
			Type:         pluginsdk.TypeInt,
			Optional:     true,
			Default:      0,
			ValidateFunc: validation.IntAtLeast(0),
			Description:  "The amount of memory in gigabyte-seconds that your application is allowed to consume per day. Setting this value only affects function apps in Consumption Plans.",
		},

		"enabled": {
			Type:        pluginsdk.TypeBool,
			Optional:    true,
			Default:     true,
			Description: "Is the Linux Function App Slot enabled.",
		},

		"content_share_force_disabled": {
			Type:        pluginsdk.TypeBool,
			Optional:    true,
			Default:     false,
			Description: "Force disable the content share settings.",
		},

		"functions_extension_version": {
			Type:        pluginsdk.TypeString,
			Optional:    true,
			Default:     "~4",
			Description: "The runtime version associated with the Function App Slot.",
		},

		"https_only": {
			Type:        pluginsdk.TypeBool,
			Optional:    true,
			Default:     false,
			Description: "Can the Function App Slot only be accessed via HTTPS?",
		},

		"identity": commonschema.SystemAssignedUserAssignedIdentityOptional(),

		"key_vault_reference_identity_id": {
			Type:         pluginsdk.TypeString,
			Optional:     true,
			Computed:     true,
			ValidateFunc: commonids.ValidateUserAssignedIdentityID,
			Description:  "The User Assigned Identity to use for Key Vault access.",
		},

		"site_config": helpers.SiteConfigSchemaLinuxFunctionAppSlot(),

		"tags": tags.Schema(),
	}
}

func (r LinuxFunctionAppSlotResource) Attributes() map[string]*pluginsdk.Schema {
	return map[string]*pluginsdk.Schema{
		"custom_domain_verification_id": {
			Type:      pluginsdk.TypeString,
			Computed:  true,
			Sensitive: true,
		},

		"default_hostname": {
			Type:     pluginsdk.TypeString,
			Computed: true,
		},

		"kind": {
			Type:     pluginsdk.TypeString,
			Computed: true,
		},

		"outbound_ip_addresses": {
			Type:     pluginsdk.TypeString,
			Computed: true,
		},

		"outbound_ip_address_list": {
			Type:     pluginsdk.TypeList,
			Computed: true,
			Elem: &pluginsdk.Schema{
				Type: pluginsdk.TypeString,
			},
		},

		"possible_outbound_ip_addresses": {
			Type:     pluginsdk.TypeString,
			Computed: true,
		},

		"possible_outbound_ip_address_list": {
			Type:     pluginsdk.TypeList,
			Computed: true,
			Elem: &pluginsdk.Schema{
				Type: pluginsdk.TypeString,
			},
		},

		"site_credential": helpers.SiteCredentialSchema(),
	}
}

func (r LinuxFunctionAppSlotResource) Create() sdk.ResourceFunc {
	return sdk.ResourceFunc{
		Timeout: 30 * time.Minute,
		Func: func(ctx context.Context, metadata sdk.ResourceMetaData) error {
			var functionAppSlot LinuxFunctionAppSlotModel

			if err := metadata.Decode(&functionAppSlot); err != nil {
				return err
			}

			client := metadata.Client.AppService.WebAppsClient
			functionAppId, err := parse.FunctionAppID(functionAppSlot.FunctionAppID)
			if err != nil {
				return err
			}

			aseClient := metadata.Client.AppService.AppServiceEnvironmentClient
			servicePlanClient := metadata.Client.AppService.ServicePlanClient
			subscriptionId := metadata.Client.Account.SubscriptionId

			id := parse.NewFunctionAppSlotID(subscriptionId, functionAppId.ResourceGroup, functionAppId.SiteName, functionAppSlot.Name)
			functionApp, err := client.Get(ctx, functionAppId.ResourceGroup, functionAppId.SiteName)
			if err != nil {
				return fmt.Errorf("retrieving parent Linux %s: %+v", *functionAppId, err)
			}
			if functionApp.Location == nil {
				return fmt.Errorf("could not determine location for %s: %+v", id, err)
			}
			props := functionApp.SiteProperties
			if props == nil || props.ServerFarmID == nil {
				return fmt.Errorf("could not determine Service Plan ID for %s: %+v", id, err)
			}
			servicePlanId, err := parse.ServicePlanID(*props.ServerFarmID)
			if err != nil {
				return err
			}

			servicePlan, err := servicePlanClient.Get(ctx, servicePlanId.ResourceGroup, servicePlanId.ServerfarmName)
			if err != nil {
				return fmt.Errorf("reading %s: %+v", servicePlanId, err)
			}

			sendContentSettings := !functionAppSlot.ForceDisableContentShare
			if planSku := servicePlan.Sku; planSku != nil && planSku.Tier != nil {
				switch tier := *planSku.Tier; strings.ToLower(tier) {
				case "dynamic": // Consumption Plan modifications to request
					sendContentSettings = false
				case "elastic": // ElasticPremium Plan modifications to request?
				case "basic": // App Service Plan modifications to request?
					sendContentSettings = false
				case "standard":
					sendContentSettings = false
				case "premiumv2", "premiumv3":
					sendContentSettings = false
				}
			} else {
				return fmt.Errorf("determining plan type for Linux %s: %v", id, err)
			}

			existing, err := client.GetSlot(ctx, id.ResourceGroup, id.SiteName, id.SlotName)
			if err != nil && !utils.ResponseWasNotFound(existing.Response) {
				return fmt.Errorf("checking for presence of existing Linux %s: %+v", id, err)
			}

			if !utils.ResponseWasNotFound(existing.Response) {
				return metadata.ResourceRequiresImport(r.ResourceType(), id)
			}

			availabilityRequest := web.ResourceNameAvailabilityRequest{
				Name: utils.String(fmt.Sprintf("%s-%s", id.SiteName, id.SlotName)),
				Type: web.CheckNameResourceTypesMicrosoftWebsites,
			}

			if ase := servicePlan.HostingEnvironmentProfile; ase != nil {
				// Attempt to check the ASE for the appropriate suffix for the name availability request.
				// This varies between internal and external ASE Types, and potentially has other names in other clouds
				// We use the "internal" as the fallback here, if we can read the ASE, we'll get the full one
				nameSuffix := "appserviceenvironment.net"
				if ase.ID != nil {
					aseId, err := parse.AppServiceEnvironmentID(*ase.ID)
					nameSuffix = fmt.Sprintf("%s.%s", aseId.HostingEnvironmentName, nameSuffix)
					if err != nil {
						metadata.Logger.Warnf("could not parse App Service Environment ID determine FQDN for name availability check, defaulting to `%s.%s.appserviceenvironment.net`", functionAppSlot.Name, servicePlanId)
					} else {
						existingASE, err := aseClient.Get(ctx, aseId.ResourceGroup, aseId.HostingEnvironmentName)
						if err != nil {
							metadata.Logger.Warnf("could not read App Service Environment to determine FQDN for name availability check, defaulting to `%s.%s.appserviceenvironment.net`", functionAppSlot.Name, servicePlanId)
						} else if props := existingASE.AppServiceEnvironment; props != nil && props.DNSSuffix != nil && *props.DNSSuffix != "" {
							nameSuffix = *props.DNSSuffix
						}
					}
				}

				availabilityRequest.Name = utils.String(fmt.Sprintf("%s.%s", functionAppSlot.Name, nameSuffix))
				availabilityRequest.IsFqdn = utils.Bool(true)
			}

			checkName, err := client.CheckNameAvailability(ctx, availabilityRequest)
			if err != nil {
				return fmt.Errorf("checking name availability for Linux %s: %+v", id, err)
			}
			if checkName.NameAvailable != nil && !*checkName.NameAvailable {
				return fmt.Errorf("the Site Name %q failed the availability check: %+v", id.SiteName, *checkName.Message)
			}

			storageString := functionAppSlot.StorageAccountName
			if !functionAppSlot.StorageUsesMSI {
				if functionAppSlot.StorageKeyVaultSecretID != "" {
					storageString = fmt.Sprintf(helpers.StorageStringFmtKV, functionAppSlot.StorageKeyVaultSecretID)
				} else {
					storageString = fmt.Sprintf(helpers.StorageStringFmt, functionAppSlot.StorageAccountName, functionAppSlot.StorageAccountKey, metadata.Client.Account.Environment.StorageEndpointSuffix)
				}
			}
			siteConfig, err := helpers.ExpandSiteConfigLinuxFunctionAppSlot(functionAppSlot.SiteConfig, nil, metadata, functionAppSlot.FunctionExtensionsVersion, storageString, functionAppSlot.StorageUsesMSI)
			if err != nil {
				return fmt.Errorf("expanding site_config for Linux %s: %+v", id, err)
			}

			if functionAppSlot.BuiltinLogging {
				if functionAppSlot.AppSettings == nil {
					functionAppSlot.AppSettings = make(map[string]string)
				}
				if !functionAppSlot.StorageUsesMSI {
					functionAppSlot.AppSettings["AzureWebJobsDashboard"] = storageString
				} else {
					functionAppSlot.AppSettings["AzureWebJobsDashboard__accountName"] = functionAppSlot.StorageAccountName
				}
			}

			if sendContentSettings {
				if functionAppSlot.AppSettings == nil {
					functionAppSlot.AppSettings = make(map[string]string)
				}
				suffix := uuid.New().String()[0:4]
				if _, present := functionAppSlot.AppSettings["WEBSITE_CONTENTSHARE"]; !present {
					functionAppSlot.AppSettings["WEBSITE_CONTENTSHARE"] = fmt.Sprintf("%s-%s", strings.ToLower(functionAppSlot.Name), suffix)
				}
				if _, present := functionAppSlot.AppSettings["WEBSITE_CONTENTAZUREFILECONNECTIONSTRING"]; !present {
					functionAppSlot.AppSettings["WEBSITE_CONTENTAZUREFILECONNECTIONSTRING"] = storageString
				}
			}

			siteConfig.LinuxFxVersion = helpers.EncodeFunctionAppLinuxFxVersion(functionAppSlot.SiteConfig[0].ApplicationStack)
			siteConfig.AppSettings = helpers.MergeUserAppSettings(siteConfig.AppSettings, functionAppSlot.AppSettings)

			expandedIdentity, err := expandIdentity(metadata.ResourceData.Get("identity").([]interface{}))
			if err != nil {
				return fmt.Errorf("expanding `identity`: %+v", err)
			}

			siteEnvelope := web.Site{
				Location: functionApp.Location,
				Tags:     tags.FromTypedObject(functionAppSlot.Tags),
				Kind:     utils.String("functionapp,linux"),
				Identity: expandedIdentity,
				SiteProperties: &web.SiteProperties{
					ServerFarmID:         utils.String(servicePlanId.ID()),
					Enabled:              utils.Bool(functionAppSlot.Enabled),
					HTTPSOnly:            utils.Bool(functionAppSlot.HttpsOnly),
					SiteConfig:           siteConfig,
					ClientCertEnabled:    utils.Bool(functionAppSlot.ClientCertEnabled),
					ClientCertMode:       web.ClientCertMode(functionAppSlot.ClientCertMode),
					DailyMemoryTimeQuota: utils.Int32(int32(functionAppSlot.DailyMemoryTimeQuota)), // TODO - Investigate, setting appears silently ignored on Linux Function Apps?
				},
			}

			if functionAppSlot.KeyVaultReferenceIdentityID != "" {
				siteEnvelope.SiteProperties.KeyVaultReferenceIdentity = utils.String(functionAppSlot.KeyVaultReferenceIdentityID)
			}

			future, err := client.CreateOrUpdateSlot(ctx, id.ResourceGroup, id.SiteName, siteEnvelope, id.SlotName)
			if err != nil {
				return fmt.Errorf("creating Linux %s: %+v", id, err)
			}

			if err := future.WaitForCompletionRef(ctx, client.Client); err != nil {
				return fmt.Errorf("waiting for creation of Linux %s: %+v", id, err)
			}

			updateFuture, err := client.CreateOrUpdateSlot(ctx, id.ResourceGroup, id.SiteName, siteEnvelope, id.SlotName)
			if err != nil {
				return fmt.Errorf("updating properties of Linux %s: %+v", id, err)
			}
			if err := updateFuture.WaitForCompletionRef(ctx, client.Client); err != nil {
				return fmt.Errorf("waiting for creation of Linux %s: %+v", id, err)
			}

			backupConfig := helpers.ExpandBackupConfig(functionAppSlot.Backup)
			if backupConfig.BackupRequestProperties != nil {
				if _, err := client.UpdateBackupConfigurationSlot(ctx, id.ResourceGroup, id.SiteName, *backupConfig, id.SlotName); err != nil {
					return fmt.Errorf("adding Backup Settings for Linux %s: %+v", id, err)
				}
			}

			auth := helpers.ExpandAuthSettings(functionAppSlot.AuthSettings)
			if auth.SiteAuthSettingsProperties != nil {
				if _, err := client.UpdateAuthSettingsSlot(ctx, id.ResourceGroup, id.SiteName, *auth, id.SlotName); err != nil {
					return fmt.Errorf("setting Authorisation Settings for Linux %s: %+v", id, err)
				}
			}

			connectionStrings := helpers.ExpandConnectionStrings(functionAppSlot.ConnectionStrings)
			if connectionStrings.Properties != nil {
				if _, err := client.UpdateConnectionStringsSlot(ctx, id.ResourceGroup, id.SiteName, *connectionStrings, id.SlotName); err != nil {
					return fmt.Errorf("setting Connection Strings for Linux %s: %+v", id, err)
				}
			}

			if _, ok := metadata.ResourceData.GetOk("site_config.0.app_service_logs"); ok {
				appServiceLogs := helpers.ExpandFunctionAppAppServiceLogs(functionAppSlot.SiteConfig[0].AppServiceLogs)
				if _, err := client.UpdateDiagnosticLogsConfigSlot(ctx, id.ResourceGroup, id.SiteName, appServiceLogs, id.SlotName); err != nil {
					return fmt.Errorf("updating App Service Log Settings for %s: %+v", id, err)
				}
			}

			metadata.SetID(id)
			return nil
		},
	}
}

func (r LinuxFunctionAppSlotResource) Read() sdk.ResourceFunc {
	return sdk.ResourceFunc{
		Timeout: 5 * time.Minute,
		Func: func(ctx context.Context, metadata sdk.ResourceMetaData) error {
			client := metadata.Client.AppService.WebAppsClient
			id, err := parse.FunctionAppSlotID(metadata.ResourceData.Id())
			if err != nil {
				return err
			}
			functionApp, err := client.GetSlot(ctx, id.ResourceGroup, id.SiteName, id.SlotName)
			if err != nil {
				if utils.ResponseWasNotFound(functionApp.Response) {
					return metadata.MarkAsGone(id)
				}
				return fmt.Errorf("reading Linux %s: %+v", id, err)
			}

			if functionApp.SiteProperties == nil {
				return fmt.Errorf("reading properties of Linux %s", id)
			}
			props := *functionApp.SiteProperties

			appSettingsResp, err := client.ListApplicationSettingsSlot(ctx, id.ResourceGroup, id.SiteName, id.SlotName)
			if err != nil {
				return fmt.Errorf("reading App Settings for Linux %s: %+v", id, err)
			}

			connectionStrings, err := client.ListConnectionStringsSlot(ctx, id.ResourceGroup, id.SiteName, id.SlotName)
			if err != nil {
				return fmt.Errorf("reading Connection String information for Linux %s: %+v", id, err)
			}

			siteCredentialsFuture, err := client.ListPublishingCredentialsSlot(ctx, id.ResourceGroup, id.SiteName, id.SlotName)
			if err != nil {
				return fmt.Errorf("listing Site Publishing Credential information for Linux %s: %+v", id, err)
			}

			if err := siteCredentialsFuture.WaitForCompletionRef(ctx, client.Client); err != nil {
				return fmt.Errorf("waiting for Site Publishing Credential information for Linux %s: %+v", id, err)
			}
			siteCredentials, err := siteCredentialsFuture.Result(*client)
			if err != nil {
				return fmt.Errorf("reading Site Publishing Credential information for Linux %s: %+v", id, err)
			}

			auth, err := client.GetAuthSettingsSlot(ctx, id.ResourceGroup, id.SiteName, id.SlotName)
			if err != nil {
				return fmt.Errorf("reading Auth Settings for Linux %s: %+v", id, err)
			}

			backup, err := client.GetBackupConfigurationSlot(ctx, id.ResourceGroup, id.SiteName, id.SlotName)
			if err != nil {
				if !utils.ResponseWasNotFound(backup.Response) {
					return fmt.Errorf("reading Backup Settings for Linux %s: %+v", id, err)
				}
			}

			logs, err := client.GetDiagnosticLogsConfigurationSlot(ctx, id.ResourceGroup, id.SiteName, id.SlotName)
			if err != nil {
				return fmt.Errorf("reading logs configuration for Linux %s: %+v", id, err)
			}

			state := LinuxFunctionAppSlotModel{
				Name:                        id.SlotName,
				FunctionAppID:               parse.NewFunctionAppID(id.SubscriptionId, id.ResourceGroup, id.SiteName).ID(),
				Enabled:                     utils.NormaliseNilableBool(functionApp.Enabled),
				ClientCertMode:              string(functionApp.ClientCertMode),
				DailyMemoryTimeQuota:        int(utils.NormaliseNilableInt32(props.DailyMemoryTimeQuota)),
				Tags:                        tags.ToTypedObject(functionApp.Tags),
				Kind:                        utils.NormalizeNilableString(functionApp.Kind),
				KeyVaultReferenceIdentityID: utils.NormalizeNilableString(props.KeyVaultReferenceIdentity),
				CustomDomainVerificationId:  utils.NormalizeNilableString(props.CustomDomainVerificationID),
				DefaultHostname:             utils.NormalizeNilableString(props.DefaultHostName),
			}

			configResp, err := client.GetConfigurationSlot(ctx, id.ResourceGroup, id.SiteName, id.SlotName)
			if err != nil {
				return fmt.Errorf("making Read request on AzureRM Function App Configuration %q: %+v", id.SiteName, err)
			}

			siteConfig, err := helpers.FlattenSiteConfigLinuxFunctionAppSlot(configResp.SiteConfig)
			if err != nil {
				return fmt.Errorf("reading Site Config for Linux %s: %+v", id, err)
			}
			state.SiteConfig = []helpers.SiteConfigLinuxFunctionAppSlot{*siteConfig}

			state.unpackLinuxFunctionAppSettings(appSettingsResp, metadata)

			state.ConnectionStrings = helpers.FlattenConnectionStrings(connectionStrings)

			state.SiteCredentials = helpers.FlattenSiteCredentials(siteCredentials)

			state.AuthSettings = helpers.FlattenAuthSettings(auth)

			state.Backup = helpers.FlattenBackupConfig(backup)

			state.SiteConfig[0].AppServiceLogs = helpers.FlattenFunctionAppAppServiceLogs(logs)

			state.HttpsOnly = utils.NormaliseNilableBool(functionApp.HTTPSOnly)
			state.ClientCertEnabled = utils.NormaliseNilableBool(functionApp.ClientCertEnabled)

			if err := metadata.Encode(&state); err != nil {
				return fmt.Errorf("encoding: %+v", err)
			}

			flattenedIdentity, err := flattenIdentity(functionApp.Identity)
			if err != nil {
				return fmt.Errorf("flattening `identity`: %+v", err)
			}
			if err := metadata.ResourceData.Set("identity", flattenedIdentity); err != nil {
				return fmt.Errorf("setting `identity`: %+v", err)
			}

			return nil
		},
	}
}

func (r LinuxFunctionAppSlotResource) Delete() sdk.ResourceFunc {
	return sdk.ResourceFunc{
		Timeout: 30 * time.Minute,
		Func: func(ctx context.Context, metadata sdk.ResourceMetaData) error {
			client := metadata.Client.AppService.WebAppsClient
			id, err := parse.FunctionAppSlotID(metadata.ResourceData.Id())
			if err != nil {
				return err
			}

			metadata.Logger.Infof("deleting Linux %s", *id)

			deleteMetrics := true
			deleteEmptyServerFarm := false
			if _, err := client.DeleteSlot(ctx, id.ResourceGroup, id.SiteName, id.SlotName, &deleteMetrics, &deleteEmptyServerFarm); err != nil {
				return fmt.Errorf("deleting Linux %s: %+v", id, err)
			}
			return nil
		},
	}
}

func (r LinuxFunctionAppSlotResource) Update() sdk.ResourceFunc {
	return sdk.ResourceFunc{
		Timeout: 30 * time.Minute,
		Func: func(ctx context.Context, metadata sdk.ResourceMetaData) error {
			client := metadata.Client.AppService.WebAppsClient

			id, err := parse.FunctionAppSlotID(metadata.ResourceData.Id())
			if err != nil {
				return err
			}

			var state LinuxFunctionAppSlotModel
			if err := metadata.Decode(&state); err != nil {
				return fmt.Errorf("decoding: %+v", err)
			}

			existing, err := client.GetSlot(ctx, id.ResourceGroup, id.SiteName, id.SlotName)
			if err != nil {
				return fmt.Errorf("reading Linux %s: %v", id, err)
			}

			_, planSKU, err := helpers.ServicePlanInfoForApp(ctx, metadata, *id)
			if err != nil {
				return err
			}

			sendContentSettings := !helpers.PlanIsElastic(planSKU)

			if metadata.ResourceData.HasChange("enabled") {
				existing.SiteProperties.Enabled = utils.Bool(state.Enabled)
			}

			if metadata.ResourceData.HasChange("https_only") {
				existing.SiteProperties.HTTPSOnly = utils.Bool(state.HttpsOnly)
			}

			if metadata.ResourceData.HasChange("client_certificate_enabled") {
				existing.SiteProperties.ClientCertEnabled = utils.Bool(state.ClientCertEnabled)
			}

			if metadata.ResourceData.HasChange("client_certificate_mode") {
				existing.SiteProperties.ClientCertMode = web.ClientCertMode(state.ClientCertMode)
			}

			if metadata.ResourceData.HasChange("identity") {
				expandedIdentity, err := expandIdentity(metadata.ResourceData.Get("identity").([]interface{}))
				if err != nil {
					return fmt.Errorf("expanding `identity`: %+v", err)
				}
				existing.Identity = expandedIdentity
			}

			if metadata.ResourceData.HasChange("key_vault_reference_identity_id") {
				existing.KeyVaultReferenceIdentity = utils.String(state.KeyVaultReferenceIdentityID)
			}

			if metadata.ResourceData.HasChange("tags") {
				existing.Tags = tags.FromTypedObject(state.Tags)
			}

			storageString := state.StorageAccountName
			if !state.StorageUsesMSI {
				if state.StorageKeyVaultSecretID != "" {
					storageString = fmt.Sprintf(helpers.StorageStringFmtKV, state.StorageKeyVaultSecretID)
				} else {
					storageString = fmt.Sprintf(helpers.StorageStringFmt, state.StorageAccountName, state.StorageAccountKey, metadata.Client.Account.Environment.StorageEndpointSuffix)
				}
			}

			if sendContentSettings {
				appSettingsResp, err := client.ListApplicationSettings(ctx, id.ResourceGroup, id.SiteName)
				if err != nil {
					return fmt.Errorf("reading App Settings for Windows %s: %+v", id, err)
				}
				if state.AppSettings == nil {
					state.AppSettings = make(map[string]string)
				}
				state.AppSettings = helpers.ParseContentSettings(appSettingsResp, state.AppSettings)
			}

			// Note: We process this regardless to give us a "clean" view of service-side app_settings, so we can reconcile the user-defined entries later
			siteConfig, err := helpers.ExpandSiteConfigLinuxFunctionAppSlot(state.SiteConfig, existing.SiteConfig, metadata, state.FunctionExtensionsVersion, storageString, state.StorageUsesMSI)
			if state.BuiltinLogging {
				if state.AppSettings == nil && !state.StorageUsesMSI {
					state.AppSettings = make(map[string]string)
				}
				if !state.StorageUsesMSI {
					state.AppSettings["AzureWebJobsDashboard"] = storageString
				} else {
					state.AppSettings["AzureWebJobsDashboard__accountName"] = state.StorageAccountName
				}

			}

			if metadata.ResourceData.HasChange("site_config") {
				if err != nil {
					return fmt.Errorf("expanding Site Config for Linux %s: %+v", id, err)
				}
				existing.SiteConfig = siteConfig
			}

			if metadata.ResourceData.HasChange("site_config.0.application_stack") {
				existing.SiteConfig.LinuxFxVersion = helpers.EncodeFunctionAppLinuxFxVersion(state.SiteConfig[0].ApplicationStack)
			}

			existing.SiteConfig.AppSettings = helpers.MergeUserAppSettings(siteConfig.AppSettings, state.AppSettings)

			updateFuture, err := client.CreateOrUpdateSlot(ctx, id.ResourceGroup, id.SiteName, existing, id.SlotName)
			if err != nil {
				return fmt.Errorf("updating Linux %s: %+v", id, err)
			}
			if err := updateFuture.WaitForCompletionRef(ctx, client.Client); err != nil {
				return fmt.Errorf("waiting to update %s: %+v", id, err)
			}

			if _, err := client.UpdateConfigurationSlot(ctx, id.ResourceGroup, id.SiteName, web.SiteConfigResource{SiteConfig: siteConfig}, id.SlotName); err != nil {
				return fmt.Errorf("updating Site Config for Linux %s: %+v", id, err)
			}

			if metadata.ResourceData.HasChange("connection_string") {
				connectionStringUpdate := helpers.ExpandConnectionStrings(state.ConnectionStrings)
				if connectionStringUpdate.Properties == nil {
					connectionStringUpdate.Properties = map[string]*web.ConnStringValueTypePair{}
				}
				if _, err := client.UpdateConnectionStringsSlot(ctx, id.ResourceGroup, id.SiteName, *connectionStringUpdate, id.SlotName); err != nil {
					return fmt.Errorf("updating Connection Strings for Linux %s: %+v", id, err)
				}
			}

			if metadata.ResourceData.HasChange("auth_settings") {
				authUpdate := helpers.ExpandAuthSettings(state.AuthSettings)
				if _, err := client.UpdateAuthSettingsSlot(ctx, id.ResourceGroup, id.SiteName, *authUpdate, id.SlotName); err != nil {
					return fmt.Errorf("updating Auth Settings for Linux %s: %+v", id, err)
				}
			}

			if metadata.ResourceData.HasChange("backup") {
				backupUpdate := helpers.ExpandBackupConfig(state.Backup)
				if backupUpdate.BackupRequestProperties == nil {
					if _, err := client.DeleteBackupConfigurationSlot(ctx, id.ResourceGroup, id.SiteName, id.SlotName); err != nil {
						return fmt.Errorf("removing Backup Settings for Linux %s: %+v", id, err)
					}
				} else {
					if _, err := client.UpdateBackupConfigurationSlot(ctx, id.ResourceGroup, id.SiteName, *backupUpdate, id.SlotName); err != nil {
						return fmt.Errorf("updating Backup Settings for Linux %s: %+v", id, err)
					}
				}
			}

			if metadata.ResourceData.HasChange("site_config.0.app_service_logs") {
				appServiceLogs := helpers.ExpandFunctionAppAppServiceLogs(state.SiteConfig[0].AppServiceLogs)
				if _, err := client.UpdateDiagnosticLogsConfigSlot(ctx, id.ResourceGroup, id.SiteName, appServiceLogs, id.SlotName); err != nil {
					return fmt.Errorf("updating App Service Log Settings for %s: %+v", id, err)
				}
			}

			return nil
		},
	}
}

func (m *LinuxFunctionAppSlotModel) unpackLinuxFunctionAppSettings(input web.StringDictionary, metadata sdk.ResourceMetaData) {
	if input.Properties == nil {
		return
	}

	appSettings := make(map[string]string)
	var dockerSettings helpers.ApplicationStackDocker
	m.BuiltinLogging = false

	for k, v := range input.Properties {
		switch k {
		case "FUNCTIONS_EXTENSION_VERSION":
			m.FunctionExtensionsVersion = utils.NormalizeNilableString(v)

		case "WEBSITE_NODE_DEFAULT_VERSION": // Note - This is only set if it's not the default of 12, but we collect it from LinuxFxVersion so can discard it here
		case "WEBSITE_CONTENTAZUREFILECONNECTIONSTRING":
			if _, ok := metadata.ResourceData.GetOk("app_settings.WEBSITE_CONTENTAZUREFILECONNECTIONSTRING"); ok {
				appSettings[k] = utils.NormalizeNilableString(v)
			}

		case "WEBSITE_CONTENTSHARE":
			if _, ok := metadata.ResourceData.GetOk("app_settings.WEBSITE_CONTENTSHARE"); ok {
				appSettings[k] = utils.NormalizeNilableString(v)
			}

		case "WEBSITE_HTTPLOGGING_RETENTION_DAYS":
		case "FUNCTIONS_WORKER_RUNTIME":
			if len(m.SiteConfig) > 0 && len(m.SiteConfig[0].ApplicationStack) == 0 {
				if *v == "custom" {
					m.SiteConfig[0].ApplicationStack = []helpers.ApplicationStackLinuxFunctionApp{{CustomHandler: true}}
				}
			}
			if _, ok := metadata.ResourceData.GetOk("app_settings.FUNCTIONS_WORKER_RUNTIME"); ok {
				appSettings[k] = utils.NormalizeNilableString(v)
			}

		case "DOCKER_REGISTRY_SERVER_URL":
			dockerSettings.RegistryURL = utils.NormalizeNilableString(v)

		case "DOCKER_REGISTRY_SERVER_USERNAME":
			dockerSettings.RegistryUsername = utils.NormalizeNilableString(v)

		case "DOCKER_REGISTRY_SERVER_PASSWORD":
			dockerSettings.RegistryPassword = utils.NormalizeNilableString(v)

		// case "WEBSITES_ENABLE_APP_SERVICE_STORAGE": // TODO - Support this as a configurable bool, default `false` - Ref: https://docs.microsoft.com/en-us/azure/app-service/faq-app-service-linux#i-m-using-my-own-custom-container--i-want-the-platform-to-mount-an-smb-share-to-the---home---directory-

		case "APPINSIGHTS_INSTRUMENTATIONKEY":
			m.SiteConfig[0].AppInsightsInstrumentationKey = utils.NormalizeNilableString(v)

		case "APPLICATIONINSIGHTS_CONNECTION_STRING":
			m.SiteConfig[0].AppInsightsConnectionString = utils.NormalizeNilableString(v)

		case "AzureWebJobsStorage":
			if v != nil && strings.HasPrefix(*v, "@Microsoft.KeyVault") {
				trimmed := strings.TrimPrefix(strings.TrimSuffix(*v, ")"), "@Microsoft.KeyVault(SecretUri=")
				m.StorageKeyVaultSecretID = trimmed
			} else {
				m.StorageAccountName, m.StorageAccountKey = helpers.ParseWebJobsStorageString(v)
			}

		case "AzureWebJobsDashboard":
			m.BuiltinLogging = true

		case "WEBSITE_HEALTHCHECK_MAXPINGFAILURES":
			i, _ := strconv.Atoi(utils.NormalizeNilableString(v))
			m.SiteConfig[0].HealthCheckEvictionTime = utils.NormaliseNilableInt(&i)

		case "AzureWebJobsStorage__accountName":
			m.StorageUsesMSI = true
			m.StorageAccountName = utils.NormalizeNilableString(v)

		case "AzureWebJobsDashboard__accountName":
			m.BuiltinLogging = true

		case "WEBSITE_RUN_FROM_PACKAGE":
			if _, ok := metadata.ResourceData.GetOk("app_settings.WEBSITE_RUN_FROM_PACKAGE"); ok {
				appSettings[k] = utils.NormalizeNilableString(v)
			}

		default:
			appSettings[k] = utils.NormalizeNilableString(v)
		}
	}

	if dockerSettings.RegistryURL != "" {
		appStack := make([]helpers.ApplicationStackLinuxFunctionApp, 0)
		docker, _ := helpers.DecodeFunctionAppDockerFxString(m.SiteConfig[0].LinuxFxVersion, dockerSettings)
		appStack = append(appStack, helpers.ApplicationStackLinuxFunctionApp{Docker: docker})
		m.SiteConfig[0].ApplicationStack = appStack
	}

	m.AppSettings = appSettings
}
